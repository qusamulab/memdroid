package cli

import (
	"fmt"

	"memdroid/internal/app"
	"memdroid/internal/driver"
	"memdroid/internal/driver/adb"
	"memdroid/internal/memory/search"
)

// cmd is the context handed to a menu action. Every field is resolved fresh
// when the key is pressed, so an action never sees state left over from an
// earlier iteration of the menu loop.
type cmd struct {
	st   *app.State
	d    *adb.ADB
	drv  driver.Driver
	pid  int
	vt   search.ValueType
	sess *search.Session
}

func newCmd(st *app.State, d *adb.ADB) *cmd {
	c := &cmd{st: st, d: d, drv: st.GetDriver(), vt: st.GetValueType()}
	if target, err := st.Capture(); err == nil {
		c.drv, c.pid, c.vt, c.sess = target.Driver, target.PID, target.ValueType, target.Session
	}
	return c
}

type action func(*cmd)

// attached runs fn only when a process is attached.
func attached(fn action) action {
	return func(c *cmd) {
		if c.pid == 0 {
			fmt.Println("No process attached")
			return
		}
		fn(c)
	}
}

// withSession runs fn only when there is a search session holding candidates.
func withSession(fn action) action {
	return func(c *cmd) {
		if c.sess == nil || !c.sess.HasCandidates() {
			fmt.Println("No active search session. Run Search first.")
			return
		}
		fn(c)
	}
}

func currentTarget(st *app.State) (app.Target, bool) {
	target, err := st.Capture()
	if err != nil {
		fmt.Println(err)
		return app.Target{}, false
	}
	return target, true
}
