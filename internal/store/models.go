package store

// User is an account that can sign in. The first user is the admin; admins
// manage other users. Every user has their own shelf.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Book statuses.
const (
	StatusWant      = "want"
	StatusReading   = "reading"
	StatusRead      = "read"
	StatusAbandoned = "abandoned"
)

// ValidStatus reports whether s is a known status.
func ValidStatus(s string) bool {
	return s == StatusWant || s == StatusReading || s == StatusRead || s == StatusAbandoned
}

// Book is one book on a user's shelf, with their verdict and plan for it.
type Book struct {
	ID          int64   `json:"id"`
	UserID      int64   `json:"user_id"`
	Title       string  `json:"title"`
	Subtitle    string  `json:"subtitle"`
	Authors     string  `json:"authors"`
	ISBN13      string  `json:"isbn13"`
	ISBN10      string  `json:"isbn10"`
	Pages       int     `json:"pages"`
	Published   string  `json:"published"`
	Publisher   string  `json:"publisher"`
	Language    string  `json:"language"`
	Description string  `json:"description"`
	Format      string  `json:"format"`
	Tags        string  `json:"tags"`
	Cover       string  `json:"cover"`
	Source      string  `json:"source"`
	SourceID    string  `json:"source_id"`
	SourceURL   string  `json:"source_url"`
	Status      string  `json:"status"`
	Rating      float64 `json:"rating"`
	Review      string  `json:"review"`
	Favorite    bool    `json:"favorite"`
	CurrentPage int     `json:"current_page"`
	QueuePos    int     `json:"queue_pos"`
	PlannedFor  string  `json:"planned_for"`
	PlanNote    string  `json:"plan_note"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`

	// Derived from reads, read-only.
	StartedOn  string `json:"started_on"`  // start of the open read, else of the latest one
	FinishedOn string `json:"finished_on"` // latest finished (not abandoned) read
	ReadCount  int    `json:"read_count"`  // finished, not abandoned
	QuoteCount int    `json:"quote_count"`
}

// Read is one time through a book.
type Read struct {
	ID         int64  `json:"id"`
	BookID     int64  `json:"book_id"`
	StartedOn  string `json:"started_on"`
	FinishedOn string `json:"finished_on"`
	Abandoned  bool   `json:"abandoned"`
	CreatedAt  string `json:"created_at"`
}

// Quote is a passage saved from a book.
type Quote struct {
	ID        int64  `json:"id"`
	BookID    int64  `json:"book_id"`
	Text      string `json:"text"`
	Page      string `json:"page"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`

	// Joined, read-only.
	BookTitle   string `json:"book_title"`
	BookAuthors string `json:"book_authors"`
	BookCover   string `json:"book_cover"`
}

// Settings are preferences stored as key/value pairs, shared by everyone.
type Settings struct {
	Title          string `json:"title"`
	YearlyGoal     int    `json:"yearly_goal"` // books per year, 0 = no goal
	Landing        string `json:"landing"`     // now | library | planner
	SearchSource   string `json:"search_source"`
	GoogleBooksKey string `json:"google_books_key"`
	SetupComplete  bool   `json:"setup_complete"`
}

// Dashboard is the compact status used by widgets.
type Dashboard struct {
	Reading       int        `json:"reading"`
	ReadThisYear  int        `json:"read_this_year"`
	PagesThisYear int        `json:"pages_this_year"`
	YearlyGoal    int        `json:"yearly_goal"`
	GoalPercent   int        `json:"goal_percent"`
	Want          int        `json:"want"`
	ReadTotal     int        `json:"read_total"`
	Books         int        `json:"books"`
	Quotes        int        `json:"quotes"`
	AvgRating     float64    `json:"avg_rating"`
	CurrentlyRead []BookLine `json:"currently_reading"`
	LastFinished  *BookLine  `json:"last_finished"`
	UpNext        *BookLine  `json:"up_next"`
}

// BookLine is the short form of a book for widgets.
type BookLine struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Authors  string  `json:"authors"`
	Rating   float64 `json:"rating,omitempty"`
	Pages    int     `json:"pages,omitempty"`
	Progress int     `json:"progress,omitempty"` // percent, when pages are known
	Date     string  `json:"date,omitempty"`
}
