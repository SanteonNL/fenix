package fhirserver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/patientnumbers"
	"github.com/SanteonNL/fenix/internal/querycompiler"
)

// Defaults for Server.SetExportTuning.
const (
	defaultExportBatchSize = 1000
	defaultExportWorkers   = 4
)

func (s *Server) exportBatch() int {
	if s.exportBatchSize > 0 {
		return s.exportBatchSize
	}
	return defaultExportBatchSize
}

func (s *Server) exportParallelism() int {
	if s.exportWorkers > 0 {
		return s.exportWorkers
	}
	return defaultExportWorkers
}

var resourceTypeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// validResourceTypeName guards the name used to build the export file name.
func validResourceTypeName(s string) bool { return resourceTypeName.MatchString(s) }

// chunk splits ids into slices of at most size. There is always at least one
// chunk (empty for no ids), so an export of an empty group still runs each
// type's query once and can tell an unconfigured type from an empty result.
func chunk(ids []string, size int) [][]string {
	if len(ids) == 0 {
		return [][]string{nil}
	}
	out := make([][]string, 0, (len(ids)+size-1)/size)
	for len(ids) > size {
		out = append(out, ids[:size])
		ids = ids[size:]
	}
	return append(out, ids)
}

// exportTypeJob is one resource type's share of an export.
type exportTypeJob struct {
	resourceType   string
	filters        map[string]string        // the type's _typeFilter params
	batches        [][]string               // patient ids, per batch
	patientNumbers *patientnumbers.Registry // old -> active patient numbers; nil = none
	elements       map[string][]string      // _elements for de-identification
	runID          string
	queriesDir     string // where compiled queries go ("" = default location)
	groupFile      string
	path           string                // NDJSON file to write
	progress       func(batchesDone int) // optional; called after each batch is written
}

// batchOut is the NDJSON of one finished batch, or why it failed.
type batchOut struct {
	data   []byte
	n      int
	status int // HTTP status to report when err != nil
	err    error
}

// exportType fetches one resource type for every batch of patients and writes
// the result to job.path as NDJSON, in batch order. It returns the number of
// resources written, and the HTTP status to report when it fails.
//
// Up to exportParallelism batches are in flight at once. A batch holds its
// slot until the file writer has consumed it, so at most that many finished
// batches ever wait in memory, however slow an earlier batch is.
func (s *Server) exportType(ctx context.Context, job exportTypeJob) (count, status int, err error) {
	f, err := os.Create(job.path)
	if err != nil {
		return 0, http.StatusInternalServerError, err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, 1<<20)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	slots := make([]chan batchOut, len(job.batches))
	for i := range slots {
		slots[i] = make(chan batchOut, 1) // each is written exactly once, so a task never blocks on it
	}
	inFlight := make(chan struct{}, s.exportParallelism())

	go func() {
		for i, ids := range job.batches {
			select {
			case inFlight <- struct{}{}:
			case <-ctx.Done():
				for j := i; j < len(slots); j++ {
					slots[j] <- batchOut{err: ctx.Err(), status: http.StatusInternalServerError}
				}
				return
			}
			go func(i int, ids []string) {
				slots[i] <- s.exportBatchOf(ctx, job, i, ids)
			}(i, ids)
		}
	}()

	for i, slot := range slots {
		out := <-slot
		if out.err != nil {
			return 0, out.status, out.err // the deferred cancel stops the batches still running
		}
		if _, err := bw.Write(out.data); err != nil {
			return 0, http.StatusInternalServerError, err
		}
		count += out.n
		if job.progress != nil {
			job.progress(i + 1)
		}
		select { // free the slot; a no-op for batches the dispatcher cancelled before they took one
		case <-inFlight:
		default:
		}
	}
	if err := bw.Flush(); err != nil {
		return 0, http.StatusInternalServerError, err
	}
	return count, 0, nil
}

// exportBatchOf runs one batch: fetch the type's resources for these
// patients, de-identify them, and serialize them to NDJSON.
func (s *Server) exportBatchOf(ctx context.Context, job exportTypeJob, index int, patientIDs []string) batchOut {
	resources, found, err := s.searchInGroup(ctx, job.resourceType, job.filters, searchOpts{
		membersSQL: querycompiler.InList(patientIDs),
		queryDir:   job.queriesDir,
		groupFile:  job.groupFile,
		skipWrite:  index > 0, // every batch compiles to the same query file
	})
	if err != nil {
		return batchOut{err: err, status: http.StatusInternalServerError}
	}
	if !found {
		return batchOut{err: fmt.Errorf("resource type %q is not configured for source %q", job.resourceType, s.source), status: http.StatusBadRequest}
	}

	resources = job.patientNumbers.UseActiveNumber(resources)

	if s.deidentRuleset != nil {
		if err := s.deidentifyWithRunID(resources, job.elements, job.runID); err != nil {
			return batchOut{err: fmt.Errorf("de-identifying: %w", err), status: http.StatusUnprocessableEntity}
		}
	}

	var buf bytes.Buffer
	if err := converter.WriteNDJSON(&buf, resources); err != nil {
		return batchOut{err: fmt.Errorf("serializing: %w", err), status: http.StatusInternalServerError}
	}
	return batchOut{data: buf.Bytes(), n: len(resources)}
}
