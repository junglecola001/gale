// Package model holds the types that cross the Go/TypeScript boundary and
// that the store persists. Exported fields carry camelCase JSON tags: the
// generated TypeScript client derives its types from them.
package model

// Status is where a download stands.
type Status string

// The statuses a download can hold.
const (
	Queued      Status = "queued"
	Downloading Status = "downloading"
	Paused      Status = "paused"
	Completed   Status = "completed"
	Failed      Status = "error"
)

// Download is one download's row.
type Download struct {
	ID           int64  `json:"id"`
	URL          string `json:"url"`
	FinalURL     string `json:"finalUrl"`
	FileName     string `json:"fileName"`
	Dir          string `json:"dir"`
	TempPath     string `json:"tempPath"`
	TotalBytes   int64  `json:"totalBytes"` // -1 while unknown
	Downloaded   int64  `json:"downloaded"`
	ETag         string `json:"etag"`
	LastModified string `json:"lastModified"`
	MIME         string `json:"mime"`
	Status       Status `json:"status"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	Resumable    bool   `json:"resumable"`
	Connections  int    `json:"connections"`
	Category     string `json:"category"`
	CreatedAt    int64  `json:"createdAt"`  // unix ms
	StartedAt    int64  `json:"startedAt"`  // unix ms, 0 until it started
	FinishedAt   int64  `json:"finishedAt"` // unix ms, 0 until it finished
	// Speed and ETA are filled by progress events only; they are not stored.
	Speed int64 `json:"speed"`
	ETA   int64 `json:"eta"`

	// The rest is what the next request for the file needs, and stays in
	// Go: the page has no use for cookies.
	Referrer   string `json:"-"`
	UserAgent  string `json:"-"`
	Cookies    string `json:"-"`
	NeedsProbe bool   `json:"-"`
	AutoResume bool   `json:"-"`
}

// Segment is one range of a download: bytes [Start, End], of which
// [Start, Pos) are on disk. Index orders the segments by Start.
type Segment struct {
	Index int   `json:"index"`
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Pos   int64 `json:"pos"`
}

// Done reports whether the segment has all its bytes.
func (s Segment) Done() bool { return s.Pos > s.End }

// Remaining is how many bytes the segment has left.
func (s Segment) Remaining() int64 {
	if s.Done() {
		return 0
	}
	return s.End - s.Pos + 1
}

// AddRequest asks for a new download.
type AddRequest struct {
	URL         string `json:"url"`
	Dir         string `json:"dir"`
	FileName    string `json:"fileName"`
	Referer     string `json:"referer"`
	UserAgent   string `json:"userAgent"`
	Cookies     string `json:"cookies"`
	Category    string `json:"category"`
	Connections int    `json:"connections"` // 0 uses the setting
	Start       bool   `json:"start"`       // false adds it paused
}

// Preview is what a probe learned about a URL before it is downloaded.
type Preview struct {
	FinalURL  string `json:"finalUrl"`
	FileName  string `json:"fileName"`
	MIME      string `json:"mime"`
	Size      int64  `json:"size"` // -1 while unknown
	Resumable bool   `json:"resumable"`
}

// Progress is a running download's state, sent many times a second.
type Progress struct {
	ID         int64 `json:"id"`
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
	Speed      int64 `json:"speed"`
	ETA        int64 `json:"eta"`
}

// Finished reports a download's end: Completed or Failed.
type Finished struct {
	ID           int64  `json:"id"`
	Status       Status `json:"status"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	FilePath     string `json:"filePath"`
}

// Stats is the whole engine at a glance.
type Stats struct {
	Active int   `json:"active"`
	Queued int   `json:"queued"`
	Speed  int64 `json:"speed"`
}

// Detail is a download with its segments, for the detail drawer.
type Detail struct {
	Download Download  `json:"download"`
	Segments []Segment `json:"segments"`
}
