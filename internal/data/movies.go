package data

import (
	"time"
)

// TODO: Check of Customizing the Runtime field (p. 61 of Let's go further) is relevant for
// FHIR ouput.
// Annotate the Movie struct with struct tags to control how the keys appear in the
// JSON-encoded output.
type Movie struct {
	ID        int64     `json:"id"`                       // Unique integer ID for the movie
	CreatedAt time.Time `json:"-"`                        // Use the - directive; Timestamp for when the movie is added to our database
	Title     string    `json:"title"`                    // Movie title
	Year      int32     `json:"year,omitempty"`           // Movie release year
	Runtime   int32     `json:"runtime,omitempty,string"` // Movie runtime (in minutes)
	Genres    []string  `json:"genres,omitempty"`         // Slice of genres for the movie (romance, comedy, etc.)
	Version   int32     `json:"version"`                  // The version number starts at 1 and will be incremented each
	// time the movie information is updated
}
