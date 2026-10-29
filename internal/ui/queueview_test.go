package ui

import "testing"

const twoAlbums = "Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
	"Undertow, Quiet Room, Blue Static, Radio Silence, Homeward"

// Blue Hour plays (Dream Pop) from Dusk Signal; The Quiet Ones' Blue
// (Indie) follows.
func queueOfTwoAlbums(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>hk<CR>VGA<Tab>")
	h.queueIs(twoAlbums)
	return h
}

func TestSortQueue(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys(":sort title<CR>")
	h.contains("queue sorted by title · u undoes")
	// What plays stays first.
	h.queueIs("Dusk Signal, Afterglow, Blue Hour, Blue Static, Homeward, Low Tide, " +
		"Quiet Room, Radio Silence, Streetlamp, Undertow, Wintering")
	h.contains("▶  1 Dusk Signal")
	h.keys("u")
	h.queueIs(twoAlbums)
	h.keys(":sort<CR>")
	h.contains("the queue has no usual order · u undoes the sort")
	h.keys(":sort bogus<CR>")
	h.contains(
		`sort: "bogus" isn't a field here; try title, artist, album, year, plays, length, genre`,
	)
}

func TestFilterQueue(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys(":filter genre indie<CR>")
	h.contains("Queue (6 of 11) · genre indie", "6 shown")
	h.lacks("Low Tide", "Wintering")
	h.contains("▶  1 Dusk Signal") // playing, though it doesn't match
	h.cursorOn("Dusk Signal")

	h.keys("jjdd") // Quiet Room
	h.contains("1 track removed (10 in queue)")
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
		"Undertow, Blue Static, Radio Silence, Homeward")
	h.cursorOn("Blue Static")
	h.keys("Vjd")
	h.queueIs(
		"Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, Undertow, Homeward",
	)
	h.cursorOn("Homeward")
	h.keys("P") // back before the row shown
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
		"Undertow, Blue Static, Radio Silence, Homeward")
	h.keys("J")
	h.contains("filtered view · :filter alone to move tracks")

	h.keys("ggj<CR>") // plays the row shown
	h.contains("▶  7 Undertow")

	h.keys("G<Tab>gp") // the row that plays
	h.cursorOn("Undertow")
	h.keys(":filter year 1900<CR>")
	h.contains("Queue (1 of 10)", "▶  7 Undertow") // only what plays
	h.keys(":filter<CR>")
	h.contains("filter cleared", "Queue (10)", "Low Tide")
	h.cursorOn("Undertow") // the cursor stays on its entry
}
