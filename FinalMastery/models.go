package main

// Album represents an album record.
type Album struct {
	AlbumID     string `json:"album_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

// Photo represents a photo record.
type Photo struct {
	PhotoID string `json:"photo_id"`
	AlbumID string `json:"album_id"`
	Seq     int    `json:"seq"`
	Status  string `json:"status"`
	S3Key   string `json:"-"`         // internal only, not serialised
	URL     string `json:"url,omitempty"` // only present when completed
}

// PhotoJob is the unit of work passed from POST handlers to the worker pool.
type PhotoJob struct {
	PhotoID     string
	S3Key       string
	FileBytes   []byte
	ContentType string
}
