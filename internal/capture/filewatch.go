package capture

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/vdpeijl/clk/internal/gitctx"
	"github.com/vdpeijl/clk/internal/hookparse"
	"github.com/vdpeijl/clk/internal/sessions"
)

// defaultHeartbeatInterval is the minimum spacing between file-watch heartbeats
// for a single project. Editing fires many filesystem events per minute; one
// heartbeat per interval is enough to keep an editor-only session alive without
// flooding the store.
const defaultHeartbeatInterval = 60 * time.Second

// watchProject pairs a registered repository root with its project token.
type watchProject struct {
	root  string
	token string
}

// throttle gates events to at most one per key per interval. It is not
// goroutine-safe; the file watcher drives it from a single event loop.
type throttle struct {
	interval time.Duration
	last     map[string]time.Time
}

func newThrottle(interval time.Duration) *throttle {
	return &throttle{interval: interval, last: make(map[string]time.Time)}
}

// allow reports whether an event for key is permitted at now, recording the
// time when it is. It is pure with respect to its inputs and internal state.
func (t *throttle) allow(key string, now time.Time) bool {
	if last, ok := t.last[key]; ok && now.Sub(last) < t.interval {
		return false
	}
	t.last[key] = now
	return true
}

// ignoredDirs are directory names never worth watching: VCS metadata, generated
// output, dependency trees, and editor/IDE state.
var ignoredDirs = map[string]bool{
	".git":         true,
	".clk":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".idea":        true,
	".vscode":      true,
}

// ignoreDir reports whether a directory of the given base name should be skipped
// when adding watches.
func ignoreDir(name string) bool {
	return ignoredDirs[name]
}

// ignoreFile reports whether a file of the given base name is an editor/VCS
// scratch file whose changes should not count as activity.
func ignoreFile(name string) bool {
	if strings.HasPrefix(name, ".#") {
		return true
	}
	for _, suffix := range []string{"~", ".swp", ".swx", ".swo", ".tmp"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// projectFor returns the registered project that owns path, matching the
// longest applicable root prefix. It is pure.
func projectFor(path string, projects []watchProject) (watchProject, bool) {
	var (
		best  watchProject
		found bool
	)
	for _, p := range projects {
		if path == p.root || strings.HasPrefix(path, p.root+string(os.PathSeparator)) {
			if !found || len(p.root) > len(best.root) {
				best, found = p, true
			}
		}
	}
	return best, found
}

// fileWatcher emits throttled heartbeat events as files change under each
// registered project root. It is the I/O-heavy counterpart to the pure helpers
// above.
type fileWatcher struct {
	fsw      *fsnotify.Watcher
	projects []watchProject
	throttle *throttle
	emit     func(sessions.Event)
	logger   *log.Logger
}

// newFileWatcher builds a watcher over the given projects and adds recursive
// watches under each root.
func newFileWatcher(projects []watchProject, interval time.Duration, emit func(sessions.Event), logger *log.Logger) (*fileWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &fileWatcher{
		fsw:      fsw,
		projects: projects,
		throttle: newThrottle(interval),
		emit:     emit,
		logger:   logger,
	}
	for _, p := range projects {
		w.addTree(p.root)
	}
	return w, nil
}

// addTree adds watches to root and all of its non-ignored subdirectories.
// fsnotify is not recursive, so each directory is registered individually.
func (w *fileWatcher) addTree(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than abort the walk
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && ignoreDir(d.Name()) {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(path); err != nil {
			w.logger.Printf("filewatch: add %s: %v", path, err)
		}
		return nil
	})
}

// run consumes filesystem events until the watcher is closed.
func (w *fileWatcher) run() {
	for {
		select {
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.logger.Printf("filewatch error: %v", err)
		}
	}
}

// handle reacts to a single filesystem event: extending coverage to new
// directories and emitting a throttled heartbeat for real file edits.
func (w *fileWatcher) handle(ev fsnotify.Event) {
	// A newly created directory must be watched too, so editing files inside it
	// is captured.
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if !ignoreDir(filepath.Base(ev.Name)) {
				w.addTree(ev.Name)
			}
			return
		}
	}

	if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}
	if ignoreFile(filepath.Base(ev.Name)) {
		return
	}
	proj, ok := projectFor(ev.Name, w.projects)
	if !ok {
		return
	}
	now := time.Now()
	if !w.throttle.allow(proj.root, now) {
		return
	}
	w.emit(heartbeat(proj, now))
}

// Close stops the watcher and releases its OS resources.
func (w *fileWatcher) Close() error {
	return w.fsw.Close()
}

// heartbeat builds a file-watch event for a project, attaching current git
// context so editor-only sessions are still labelled with branch and issue id.
func heartbeat(p watchProject, now time.Time) sessions.Event {
	ctx := gitctx.Detect(p.root)
	token := p.token
	if token == "" {
		token = ctx.ProjectToken
	}
	return sessions.Event{
		Timestamp:    now,
		Type:         "file_change",
		Source:       string(hookparse.SourceFileWatch),
		ProjectToken: token,
		Branch:       ctx.Branch,
		IssueID:      ctx.IssueID,
		Description:  "file activity",
	}
}
