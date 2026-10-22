export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface KosyncAccount { username: string; created_at: string; last_seen: string }
export interface KosyncDocument {
  document: string; progress: string; percentage: number; device: string; device_id: string; timestamp: number
  title: string; authors: string; filename: string; book_id: number | null; ignored: boolean
  book_title: string; book_authors: string
}
export interface KosyncState { url: string; account: KosyncAccount | null; documents: KosyncDocument[] }
export interface APIToken { id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type Status = 'want' | 'reading' | 'read' | 'abandoned'
export type Format = '' | 'paper' | 'ebook' | 'audio'

export interface Book {
  id: number
  title: string
  subtitle: string
  authors: string
  isbn13: string
  isbn10: string
  pages: number
  published: string
  publisher: string
  language: string
  description: string
  format: Format
  tags: string
  cover: string
  source: string
  source_id: string
  source_url: string
  status: Status
  rating: number
  review: string
  favorite: boolean
  current_page: number
  queue_pos: number
  planned_for: string
  plan_note: string
  series: string
  series_no: string
  series_qid: string
  created_at: string
  updated_at: string
  started_on: string
  finished_on: string
  read_count: number
  quote_count: number
  reader_percent: number | null // latest position from an e-reader (0..1)
}

export interface Read { id: number; book_id: number; started_on: string; finished_on: string; abandoned: boolean }
export interface Quote { id: number; book_id: number; text: string; page: string; note: string; created_at: string; favorite: boolean; hidden: boolean; last_shown: string; book_title: string; book_authors: string; book_cover: string }

export interface Candidate {
  source: 'openlibrary' | 'google'
  source_id: string
  source_url: string
  title: string
  subtitle: string
  authors: string[] | null
  published: string
  pages: number
  isbn13: string
  isbn10: string
  publisher: string
  language: string
  description: string
  subjects: string[] | null
  cover_url: string
  thumb_url: string
  on_shelf?: number
  original_title?: string
  editions?: number
  languages?: string[]
}

export interface Settings {
  title: string
  yearly_goal: number
  landing: 'now' | 'library' | 'planner'
  search_source: 'all' | 'openlibrary' | 'google'
  google_books_key: string
  google_books_env: boolean
  sources: string[]
  languages: string
  thought_templates: ThoughtTemplate[]
}

export interface ThoughtTemplate { name: string; body: string }

export interface BookLine { id: number; title: string; authors: string; rating?: number; pages?: number; progress?: number; date?: string }
export interface TagCount { name: string; count: number }

export interface Stats {
  year: number
  years: number[]
  goal: number
  totals: { books: number; pages: number; rated: number; avg_rating: number }
  all_time: { books: number; pages: number; rated: number; avg_rating: number }
  abandoned: number
  months: { books: number; pages: number }[]
  ratings: number[]
  top_authors: TagCount[]
  top_tags: TagCount[]
  formats: TagCount[]
  finished: BookLine[]
  longest: BookLine | null
  quotes: number
}

export interface Dashboard {
  reading: number
  read_this_year: number
  pages_this_year: number
  yearly_goal: number
  goal_percent: number
  want: number
  read_total: number
  books: number
  quotes: number
  avg_rating: number
}

export interface Author {
  id: number
  name: string
  ol_key: string
  bio: string
  bio_full: string
  bio_source: string
  birth_date: string
  death_date: string
  photo: string
  wikipedia: string
  followed: boolean
  edited: boolean
  enriched_at: string
  created_at: string
  books: number
  read: number
  reading: number
  want: number
  avg_rating: number
  pages: number
  last_read: string
}

export interface AuthorCandidate { key: string; name: string; birth_date: string; death_date: string; top_work: string; work_count: number; subjects: string[] | null; photo_url: string; author_id?: number }

export interface DailySet { date: string; quotes: Quote[]; sent_at: string }
export interface DailySettings { enabled: boolean; count: number; time: string; channel: 'apprise' | 'ntfy' | 'webhook'; url: string }
export interface SeriesSummary { name: string; qid: string; authors: string; books: number; read: number; reading: number; want: number; covers: Book[]; last_no: string; total: number }
export interface SeriesEntry { no: string; title: string; year?: string; ol_work?: string; cover_url?: string; thumb_url?: string; book?: Book }
export interface SeriesNext { series: string; author: string; no: string; title: string; year?: string; ol_work?: string; cover_url?: string; thumb_url?: string; book?: Book; read: number; total: number }
