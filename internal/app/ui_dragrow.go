package app

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// A playlist of the sidebar may be dragged to another place in its group,
// as a list of the platform's does: a press that moves takes the row, a
// copy of it, solid and lifted, follows the pointer down the sidebar, and
// the rows between make room for where it would land.

// A dragRow is a sidebar row the pointer drags. A press that moves arms it,
// and the row shows at off from its place, hidden by its copy.
type dragRow struct {
	moved float32  // how far the pointer moved while pressing
	armed bool     // the pointer drags the row
	off   ui.Point // where the row shows, from its place
	drop  bool     // the row landed, and shows where until the next frame
	gen   int      // the drop the row's shift was last put in place for
}

// dragArm is how far a press moves before it drags the row: more than
// MyGo's dragStart, so a press that drags is no click.
const dragArm = 6

const (
	// rowPitch is a sidebar row's height and the gap after it, which a
	// dragged row moves by.
	rowPitch  = 46
	rowHeight = 42
)

// dragRowOf makes row draggable within its group of count rows, at index:
// a press that moves takes it, and letting it go moves it by as many
// places. cover and name are what its copy shows.
func (a *app) dragRowOf(row ui.Element, group string, index, count int, st *dragRow, cover, name string, onDrop func(steps int)) {
	if count < 2 {
		return
	}
	if st.drop {
		// The list has the row where it landed: its own offset has done
		// its work.
		st.drop, st.off = false, ui.Point{}
	}
	// Dragged, not Pressed: the pointer leaves the row a few DIPs into the
	// drag, and the press goes on.
	_, dy, pressing := row.Dragged()
	if pressing {
		st.moved += dy
		if !st.armed && math.Abs(float64(st.moved)) >= dragArm {
			st.armed = true
		}
	}
	if st.armed || row.Dragging() {
		// It comes to rest on the place nearest where the pointer left it,
		// so that it lands in the list where it shows, rather than between
		// two rows. It moves down the group only: across, it stays where
		// its list is.
		slots := make([]ui.Point, count)
		for i := range slots {
			slots[i] = ui.Point{Y: float32(i-index) * rowPitch}
		}
		bounds := ui.Rect{Y: -float32(index) * rowPitch, H: float32(count-1) * rowPitch}
		row.Draggable(&st.off, ui.DraggableOptions{
			Axis:   ui.Vertical,
			Bounds: &bounds,
			Rigid:  true,
			Snap:   ui.Nearest(slots...),
		})
		if st.armed {
			// Bounds is where the row shows, offset and all.
			r := row.Bounds()
			a.drag = &dragCard{group: group, at: index, cover: cover, name: name, x: r.X, y: r.Y, w: r.W}
			// The rows it passes make room for it.
			a.dragTo(group, index, count, st.off.Y)
		}
	}
	ended := false
	if _, _, letGo := row.DragEnded(); letGo {
		steps := int(math.Round(float64(st.off.Y) / rowPitch))
		steps = max(-index, min(steps, count-1-index))
		st.armed, st.moved, st.drop, ended = false, 0, true, true
		// Its copy has become the row again, where it landed: the rows
		// around it keep their places until the frame after, when the list
		// has the new order.
		a.drag = nil
		a.dragSettled = true
		if steps != 0 {
			// Once the rows are built: moving the list here would build a
			// row of it twice.
			a.dragDropLater(func() { onDrop(steps) })
		}
	}
	if !ended && !pressing && !row.Dragging() {
		st.armed, st.moved = false, 0
		a.dragEnded(group, index)
	}
	if st.armed {
		// The copy shows instead.
		row.Opacity(0)
	}
}

// dragDropLater keeps what moving the list the dragged row landed in does,
// for the group to run once its rows are built: moving it while they are
// built would build one of them twice.
func (a *app) dragDropLater(fn func()) {
	a.dragDrops = append(a.dragDrops, fn)
}

// dragDropsRun runs them.
func (a *app) dragDropsRun() {
	drops := a.dragDrops
	a.dragDrops = nil
	for _, fn := range drops {
		fn()
	}
}

// dragGroupStart clears what showed where a dragged row would land, once
// the list has it there: the rows' shifts go at once, as their places do,
// rather than springing to them.
func (a *app) dragGroupStart() {
	if !a.dragSettled {
		return
	}
	a.dragSettled = false
	a.dragMove = nil
	a.dragGen++
}

// dragEnded forgets the card and the group of the row at index, if it is
// the row a group drags.
func (a *app) dragEnded(group string, index int) {
	if d := a.dragMove; d != nil && d.group == group && d.from == index {
		a.dragMove = nil
	}
	if d := a.drag; d != nil && d.group == group && d.at == index {
		a.drag = nil
	}
}

// dragShiftRow makes room for the row a group drags, by moving this one up
// or down a place as the dragged row passes it.
func (a *app) dragShiftRow(row ui.Element, group string, index int, st *dragRow) {
	shift := a.dragShift(group, index)
	y := row.AnimateTo("drag-shift", shift, ui.Snappy)
	if st.gen != a.dragGen {
		// The list changed: this row's place answers for its shift.
		st.gen = a.dragGen
		y = row.Jump("drag-shift", shift)
	}
	row.Translate(0, y)
}

// A dragCard is the copy of a dragged row: solid, lifted over the sidebar,
// where the pointer is.
type dragCard struct {
	group       string
	at          int
	cover, name string
	x, y, w     float32
}

// dragCardLayer draws the copy of the row a group drags, over the page, and
// under the dialogs that follow it.
func (a *app) dragCardLayer(c *ui.Context) {
	d := a.drag
	if d == nil {
		return
	}
	p := a.pal
	ui.Row(c).Absolute().Left(d.x).Top(d.y).Width(d.w).Height(rowHeight).PaddingX(9).Gap(11).
		Radius(radiusRow).AlignItems(ui.Center).Background(p.sidebar).Shadow(0, 14, 30, -8, p.cardShade).
		PassThrough().Children(func() {
		a.cover(c, d.cover, 80, 28, radiusTile-1)
		ui.Text(c, d.name).FontSize(sideFont(13)).Grow(1).Shrink(1).MinWidth(0).SingleLine()
	})
}

// dragMove is the row a group drags: where it began, how many the group
// has, and where it would land.
type dragMove struct {
	group string
	from  int
	to    int
	count int
}

// dragTo tells the group of a row being dragged at index, of count, where
// it shows: off from its place.
func (a *app) dragTo(group string, index, count int, off float32) {
	to := index + int(math.Round(float64(off)/rowPitch))
	to = max(0, min(to, count-1))
	d := a.dragMove
	if d == nil || d.group != group || d.from != index {
		d = &dragMove{group: group, from: index, count: count}
		a.dragMove = d
	}
	d.to = to
}

// dragShift is how far the row at index shows from its place while another
// of its group is dragged past it: the rows between its place and where it
// would land move one way, and it, the other.
func (a *app) dragShift(group string, index int) float32 {
	d := a.dragMove
	if d == nil || d.group != group || index == d.from {
		return 0
	}
	switch {
	case d.from < index && index <= d.to:
		return -rowPitch
	case d.to <= index && index < d.from:
		return rowPitch
	}
	return 0
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
