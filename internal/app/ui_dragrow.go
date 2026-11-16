package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// A dragRow is a sidebar row the pointer may hold and drag to another place
// in its group. A press opens the row as a click does; held for a moment,
// the row follows the pointer within its group, and onDrop says how many
// places it moved.
type dragRow struct {
	at    time.Time // when the press began
	armed bool      // the press has been held long enough to drag
	off   ui.Point  // where the row shows, from its place
}

// dragHold is how long a row is held before it may be dragged, so that a
// press stays a click. Tests take it to zero.
var dragHold = 200 * time.Millisecond

// rowPitch is a sidebar row's height and the gap after it, which a dragged
// row moves by.
const rowPitch = 46

// dragRowOf makes row draggable within its group of count rows, at index:
// a long press takes it, and letting it go moves it by as many places.
func (a *app) dragRowOf(row ui.Element, index, count int, st *dragRow, onDrop func(steps int)) {
	if count < 2 {
		return
	}
	now := time.Now()
	switch {
	case row.Pressed() && st.at.IsZero():
		st.at = now
		// Held without moving, the press looks again once it is long enough.
		time.AfterFunc(dragHold, func() { a.post(func() {}) })
	case row.Pressed() && !st.armed && now.Sub(st.at) >= dragHold:
		st.armed = true
	}
	if st.armed || row.Dragging() {
		bounds := ui.Rect{Y: -float32(index) * rowPitch, H: float32(count-1) * rowPitch}
		row.Draggable(&st.off, ui.DraggableOptions{Axis: ui.Vertical, Bounds: &bounds, Rigid: true})
	}
	if _, _, ended := row.DragEnded(); ended {
		steps := int(math.Round(float64(st.off.Y) / rowPitch))
		steps = max(-index, min(steps, count-1-index))
		st.off, st.armed, st.at = ui.Point{}, false, time.Time{}
		if steps != 0 {
			onDrop(steps)
		}
	}
	if !row.Pressed() && !row.Dragging() {
		st.armed, st.at = false, time.Time{}
	}
}

// moveBiliPlaylist moves one playlist by steps within its group: the ones
// imported from an account, or the ones collected here. The other group
// keeps its places, so an imported collection stays above them.
func (a *app) moveBiliPlaylist(id int64, steps int, imported bool) {
	var slots []int
	for i := range a.biliPlaylists {
		if (a.biliPlaylists[i].Remote != 0) == imported {
			slots = append(slots, i)
		}
	}
	from := -1
	for k, i := range slots {
		if a.biliPlaylists[i].ID == id {
			from = k
			break
		}
	}
	if from < 0 || steps == 0 {
		return
	}
	to := max(0, min(from+steps, len(slots)-1))
	if to == from {
		return
	}
	items := make([]biliPlaylist, len(slots))
	for k, i := range slots {
		items[k] = a.biliPlaylists[i]
	}
	item := items[from]
	if to > from {
		copy(items[from:to], items[from+1:to+1])
	} else {
		copy(items[to+1:from+1], items[to:from])
	}
	items[to] = item
	for k, i := range slots {
		a.biliPlaylists[i] = items[k]
	}
	a.saveBiliPlaylists()
}
