package cli

import (
	"fmt"
	"strconv"

	"memdroid/internal/app"
	"memdroid/internal/memory/store"
)

func BookmarkList(st *app.State) {
	bl := st.GetBookmarks()
	if bl.Len() == 0 {
		fmt.Println("No bookmarks")
		return
	}
	vals := bl.Values(st.GetDriver(), st.GetPID())
	for i, b := range bl.Entries() {
		fmt.Printf("[%d] 0x%x  %-20s  %s = %s\n", i, b.Addr, b.Label, b.VType, vals[b.Addr])
	}
}

func ImportCT(st *app.State) {
	path := Prompt("CT file path: ")
	if path == "" {
		fmt.Println("Path required")
		return
	}
	bookmarks, err := store.ImportCT(path)
	if err != nil {
		fmt.Printf("Import failed: %v\n", err)
		return
	}
	bl := st.GetBookmarks()
	for _, b := range bookmarks {
		bl.Add(b.Addr, b.Label, b.VType)
	}
	fmt.Printf("Imported %d bookmarks from %s\n", len(bookmarks), path)
}

func addBookmark(c *cmd) {
	addr, ok := ParseAddr("Address (hex): ")
	if !ok {
		return
	}
	c.st.GetBookmarks().Add(addr, Prompt("Label: "), c.vt)
	fmt.Printf("Bookmarked 0x%x\n", addr)
}

func modifyAllBookmarks(c *cmd) {
	val, ok := ParseValue("Value: ", c.vt)
	if !ok {
		return
	}
	count := c.st.GetBookmarks().ModifyAll(c.drv, c.pid, val, c.vt)
	fmt.Printf("Modified %d bookmarks\n", count)
}

func removeBookmark(c *cmd) {
	BookmarkList(c.st)
	idx, err := strconv.Atoi(Prompt("Index to remove: "))
	if err != nil {
		fmt.Println("Invalid index")
		return
	}
	if err := c.st.GetBookmarks().Remove(idx); err != nil {
		fmt.Printf("Remove: %v\n", err)
	}
}
