package cli

import (
	"fmt"

	"memdroid/internal/app"
	"memdroid/internal/driver/adb"
)

// exitKey leaves the menu loop. It is handled by Run rather than the menu table
// because it is the one action that ends the program.
const exitKey = "0"

// Run drives the interactive menu loop until the user selects Exit, then
// releases the target process.
func Run(st *app.State, d *adb.ADB, serverURL string) {
	defer shutdown(st)
	for {
		PrintMenu(st, d, serverURL)
		if !dispatch(st, d, Prompt("")) {
			return
		}
	}
}

// dispatch handles a single menu choice. Returns false when the user chose Exit.
func dispatch(st *app.State, d *adb.ADB, choice string) bool {
	if choice == exitKey {
		return false
	}
	fn, ok := actions[choice]
	if !ok {
		fmt.Println("Invalid choice")
		return true
	}
	fn(newCmd(st, d))
	return true
}

// shutdown releases every resource held on the device before exiting.
func shutdown(st *app.State) {
	st.Freezer.UnfreezeAll()
	st.Watcher.UnwatchAll()
	st.AlertWatcher.RemoveAll()
	if pid := st.GetPID(); pid != 0 {
		st.GetDriver().Detach(pid)
	}
	fmt.Println("Bye!")
}
