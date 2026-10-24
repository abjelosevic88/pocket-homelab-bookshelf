-- Books read on an e-reader (added or matched by reader sync) are e-books,
-- where no format was set.
UPDATE books SET format = 'ebook'
WHERE format = '' AND (source = 'kosync' OR id IN (SELECT book_id FROM kosync_documents WHERE book_id IS NOT NULL));
