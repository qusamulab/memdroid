package cli

import (
	"fmt"

	"memdroid/internal/app"
	"memdroid/internal/memory/store"
)

func saveSession(st *app.State) {
	path := promptPath("Save file")
	if err := st.Save(path); err != nil {
		fmt.Printf("Save failed: %v\n", err)
		return
	}
	fmt.Printf("Saved to %s\n", path)
}

func loadSession(st *app.State) {
	path := promptPath("Load file")
	if _, err := st.Load(path); err != nil {
		fmt.Printf("Load failed: %v\n", err)
		return
	}
	fmt.Printf("Loaded from %s\n", path)
}

func promptPath(label string) string {
	path := Prompt(fmt.Sprintf("%s [default: %s]: ", label, store.DefaultStateFile))
	if path == "" {
		return store.DefaultStateFile
	}
	return path
}
