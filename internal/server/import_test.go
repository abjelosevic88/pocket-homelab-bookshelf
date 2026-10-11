package server

import (
	"strings"
	"testing"
)

func TestBookFromGoodreadsAndStoryGraphRows(t *testing.T) {
	mk := func(header string) func([]string, ...string) string {
		col := map[string]int{}
		for i, h := range strings.Split(header, "|") {
			col[strings.ToLower(h)] = i
		}
		return func(rec []string, names ...string) string {
			for _, n := range names {
				if i, ok := col[n]; ok && i < len(rec) {
					v := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(rec[i]), "=\""), "\"")
					if v != "" {
						return v
					}
				}
			}
			return ""
		}
	}
	gr := mk("Title|Author|Additional Authors|ISBN|ISBN13|My Rating|Binding|Number of Pages|Original Publication Year|Date Read|Date Added|Bookshelves|Exclusive Shelf|My Review")
	b, reads := bookFromCSV([]string{"The Hobbit", "J.R.R. Tolkien", "Alan Lee", `="0618260307"`, `="9780618260300"`, "5", "Paperback", "366", "1937", "2019/05/14", "2019/04/01", "fantasy, read", "read", "Loved<br/>it"}, gr)
	if b.Status != "read" || b.Rating != 5 || b.Authors != "J.R.R. Tolkien, Alan Lee" || b.ISBN13 != "9780618260300" || b.Format != "paper" || b.Tags != "fantasy" || b.Review != "Loved\nit" {
		t.Fatalf("%+v", b)
	}
	if len(reads) != 1 || reads[0].FinishedOn != "2019-05-14" || b.CreatedAt != "2019-04-01T12:00:00Z" {
		t.Fatalf("%+v %s", reads, b.CreatedAt)
	}
	sg := mk("Title|Authors|ISBN/UID|Format|Read Status|Last Date Read|Star Rating|Review|Tags")
	b, reads = bookFromCSV([]string{"Circe", "Madeline Miller", "9780316556347", "digital", "did-not-finish", "2024/02/03", "3.75", "", "myth"}, sg)
	if b.Status != "abandoned" || b.Rating != 4 || b.Format != "ebook" || b.Tags != "myth" || len(reads) != 1 || !reads[0].Abandoned {
		t.Fatalf("%+v %+v", b, reads)
	}
}
