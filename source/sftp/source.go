package sftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SanteonNL/fenix/source"
	"github.com/SanteonNL/fenix/source/local"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/sftp"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"
)

// Source downloads CSV/JSON files from a remote SFTP directory and loads them
// into the staging database using the same table conventions as the local source.
// Authentication is done with an SSH private key file.
type Source struct {
	name        string
	host        string
	port        int
	username    string
	keyFile     string
	remoteDir   string
	delimiter   rune
	jsonOptions map[string]local.JSONFileConfig
	fileWriter  source.FileWriter
	log         zerolog.Logger
}

// SetFileWriter configures file-based staging output alongside the database.
func (s *Source) SetFileWriter(w source.FileWriter) {
	s.fileWriter = w
}

func New(name, host string, port int, username, keyFile, remoteDir string, delimiter rune, jsonOptions map[string]local.JSONFileConfig, log zerolog.Logger) *Source {
	if delimiter == 0 {
		delimiter = ','
	}
	if port == 0 {
		port = 22
	}
	return &Source{
		name:        name,
		host:        host,
		port:        port,
		username:    username,
		keyFile:     keyFile,
		remoteDir:   remoteDir,
		delimiter:   delimiter,
		jsonOptions: jsonOptions,
		log:         log,
	}
}

func (s *Source) Load(ctx context.Context, db *sqlx.DB) error {
	client, cleanup, err := s.connect()
	if err != nil {
		return fmt.Errorf("sftp source %q: connect: %w", s.name, err)
	}
	defer cleanup()

	tmpDir, err := os.MkdirTemp("", "fenix-sftp-*")
	if err != nil {
		return fmt.Errorf("sftp source %q: create temp dir: %w", s.name, err)
	}
	defer os.RemoveAll(tmpDir)

	if err := s.downloadDir(client, s.remoteDir, tmpDir); err != nil {
		return fmt.Errorf("sftp source %q: download: %w", s.name, err)
	}

	ls := local.New(s.name, tmpDir, s.delimiter, s.jsonOptions, s.log)
	if s.fileWriter != nil {
		ls.SetFileWriter(s.fileWriter)
	}
	return ls.Load(ctx, db)
}

func (s *Source) connect() (*sftp.Client, func(), error) {
	keyBytes, err := os.ReadFile(s.keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read key file %q: %w", s.keyFile, err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse private key: %w", err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            s.username,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // test env; replace with known_hosts in prod
	}

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	conn, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("sftp client: %w", err)
	}

	return client, func() { client.Close(); conn.Close() }, nil
}

func (s *Source) downloadDir(client *sftp.Client, remoteDir, localDir string) error {
	entries, err := client.ReadDir(remoteDir)
	if err != nil {
		return fmt.Errorf("read remote dir %q: %w", remoteDir, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".csv" && ext != ".json" {
			continue
		}

		remotePath := remoteDir + "/" + e.Name()
		localPath := filepath.Join(localDir, e.Name())

		if err := s.downloadFile(client, remotePath, localPath); err != nil {
			s.log.Error().Err(err).Str("file", e.Name()).Msg("sftp: download failed")
		}
	}
	return nil
}

func (s *Source) downloadFile(client *sftp.Client, remotePath, localPath string) error {
	src, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote %q: %w", remotePath, err)
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local %q: %w", localPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy %q: %w", remotePath, err)
	}
	s.log.Debug().Str("file", remotePath).Msg("sftp: downloaded")
	return nil
}

func constructor(name string, config map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	host, _ := config["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("sftp source %q: missing 'host'", name)
	}

	username, _ := config["username"].(string)
	if username == "" {
		return nil, fmt.Errorf("sftp source %q: missing 'username'", name)
	}

	keyFile, _ := config["key_file"].(string)
	if keyFile == "" {
		return nil, fmt.Errorf("sftp source %q: missing 'key_file'", name)
	}

	remoteDir, _ := config["remote_dir"].(string)
	if remoteDir == "" {
		remoteDir = "/"
	}

	port := 22
	if p, ok := config["port"].(int); ok && p > 0 {
		port = p
	}

	delimiter := ','
	if delim, ok := config["delimiter"].(string); ok && delim != "" {
		delimiter = rune(delim[0])
	}

	jsonOptions := local.ParseJSONOptions(config)
	return New(name, host, port, username, keyFile, remoteDir, delimiter, jsonOptions, log), nil
}

func init() {
	source.Register("sftp", constructor)
}
