//go:build js && wasm

package main

import (
	"math/rand"
	"syscall/js"
)

const (
	GridSize  = 8
	CellSize  = 52
	GridPad   = 20
	PieceArea = 140
)

var colors = []string{
	"#FF4D6D", "#FF9F1C", "#FFBF69", "#2EC4B6",
	"#3A86FF", "#8338EC", "#06D6A0", "#FB5607",
}

type Cell struct {
	Filled bool
	Color  string
}

type Piece struct {
	Cells [][]bool
	Color string
	X, Y  float64 // drag position
}

type Game struct {
	Grid      [GridSize][GridSize]Cell
	Score     int
	BestScore int
	Pieces    [3]*Piece
	Dragging  int // index of dragging piece, -1 if none
	DragOffX  float64
	DragOffY  float64
	MouseX    float64
	MouseY    float64
	GameOver  bool
	Animating []AnimCell
	Canvas    js.Value
	Ctx       js.Value
}

type AnimCell struct {
	X, Y  int
	Alpha float64
	Color string
}

var pieceShapes = [][][]bool{
	{{true}},
	{{true, true}},
	{{true}, {true}},
	{{true, true, true}},
	{{true}, {true}, {true}},
	{{true, true}, {true, true}},
	{{true, true, true}, {true, false, false}},
	{{true, true, true}, {false, false, true}},
	{{true, false}, {true, true}},
	{{false, true}, {true, true}},
	{{true, true}, {true, false}},
	{{true, true}, {false, true}},
	{{true, true, true}, {false, true, false}},
	{{false, true, false}, {true, true, true}},
	{{true, false}, {true, false}, {true, true}},
	{{false, true}, {false, true}, {true, true}},
	{{true, true}, {true, false}, {true, false}},
	{{true, true}, {false, true}, {false, true}},
	{{true, true, true, true}},
	{{true}, {true}, {true}, {true}},
	{{true, true, true}, {true, false, false}, {true, false, false}},
	{{true, true, true}, {false, false, true}, {false, false, true}},
}

func NewGame() *Game {
	g := &Game{Dragging: -1}
	g.generatePieces()
	return g
}

func (g *Game) generatePieces() {
	for i := 0; i < 3; i++ {
		if g.Pieces[i] == nil {
			g.Pieces[i] = g.randomPiece()
		}
	}
}

func (g *Game) randomPiece() *Piece {
	shape := pieceShapes[rand.Intn(len(pieceShapes))]
	color := colors[rand.Intn(len(colors))]
	return &Piece{Cells: shape, Color: color}
}

func (g *Game) pieceWidth(p *Piece) int {
	if len(p.Cells) == 0 {
		return 0
	}
	return len(p.Cells[0])
}

func (g *Game) pieceHeight(p *Piece) int {
	return len(p.Cells)
}

func (g *Game) canPlace(p *Piece, gx, gy int) bool {
	for row := range p.Cells {
		for col := range p.Cells[row] {
			if p.Cells[row][col] {
				nx, ny := gx+col, gy+row
				if nx < 0 || nx >= GridSize || ny < 0 || ny >= GridSize {
					return false
				}
				if g.Grid[ny][nx].Filled {
					return false
				}
			}
		}
	}
	return true
}

func (g *Game) placePiece(p *Piece, gx, gy int) {
	for row := range p.Cells {
		for col := range p.Cells[row] {
			if p.Cells[row][col] {
				g.Grid[gy+row][gx+col] = Cell{Filled: true, Color: p.Color}
			}
		}
	}
}

func (g *Game) clearLines() int {
	cleared := 0
	toAnim := []AnimCell{}

	// check rows
	for row := 0; row < GridSize; row++ {
		full := true
		for col := 0; col < GridSize; col++ {
			if !g.Grid[row][col].Filled {
				full = false
				break
			}
		}
		if full {
			cleared++
			for col := 0; col < GridSize; col++ {
				toAnim = append(toAnim, AnimCell{X: col, Y: row, Alpha: 1.0, Color: g.Grid[row][col].Color})
				g.Grid[row][col] = Cell{}
			}
		}
	}
	// check cols
	for col := 0; col < GridSize; col++ {
		full := true
		for row := 0; row < GridSize; row++ {
			if !g.Grid[row][col].Filled {
				full = false
				break
			}
		}
		if full {
			cleared++
			for row := 0; row < GridSize; row++ {
				toAnim = append(toAnim, AnimCell{X: col, Y: row, Alpha: 1.0, Color: g.Grid[row][col].Color})
				g.Grid[row][col] = Cell{}
			}
		}
	}

	g.Animating = append(g.Animating, toAnim...)
	return cleared
}

func (g *Game) checkGameOver() bool {
	for i, p := range g.Pieces {
		if p == nil {
			continue
		}
		_ = i
		for gy := 0; gy < GridSize; gy++ {
			for gx := 0; gx < GridSize; gx++ {
				if g.canPlace(p, gx, gy) {
					return false
				}
			}
		}
	}
	return true
}

func (g *Game) canvasXY() (float64, float64) {
	rect := g.Canvas.Call("getBoundingClientRect")
	return rect.Get("left").Float(), rect.Get("top").Float()
}

func (g *Game) gridOrigin() (float64, float64) {
	cw := g.Canvas.Get("width").Float()
	gridPx := float64(GridSize)*CellSize + float64(GridPad)*2
	ox := (cw - gridPx) / 2
	return ox + GridPad, 80
}

func (g *Game) pieceSlotPos(i int) (float64, float64) {
	cw := g.Canvas.Get("width").Float()
	ch := g.Canvas.Get("height").Float()
	slotW := cw / 3
	cx := slotW*float64(i) + slotW/2
	cy := ch - PieceArea/2 - 10
	return cx, cy
}

func (g *Game) pixelToGrid(px, py float64) (int, int) {
	ox, oy := g.gridOrigin()
	gx := int((px - ox) / CellSize)
	gy := int((py - oy) / CellSize)
	return gx, gy
}

// ---- Render ----

func (g *Game) render(_ js.Value, _ []js.Value) interface{} {
	ctx := g.Ctx
	cw := g.Canvas.Get("width").Float()
	ch := g.Canvas.Get("height").Float()

	// background
	ctx.Set("fillStyle", "#0f0e17")
	ctx.Call("fillRect", 0, 0, cw, ch)

	// decorative bg dots
	ctx.Set("fillStyle", "rgba(255,255,255,0.03)")
	for row := 0; row < 20; row++ {
		for col := 0; col < 12; col++ {
			ctx.Call("beginPath")
			ctx.Call("arc", float64(col)*36+18, float64(row)*36+18, 2, 0, 6.2832)
			ctx.Call("fill")
		}
	}

	g.drawScore()
	g.drawGrid()
	g.drawPieces()
	g.drawDragging()
	g.drawAnimations()

	if g.GameOver {
		g.drawGameOver()
	}

	return nil
}

func (g *Game) drawScore() {
	ctx := g.Ctx
	cw := g.Canvas.Get("width").Float()

	ctx.Set("fillStyle", "rgba(255,255,255,0.06)")
	roundRect(ctx, cw/2-90, 14, 180, 52, 16)
	ctx.Call("fill")

	ctx.Set("font", "bold 28px 'Space Mono', monospace")
	ctx.Set("fillStyle", "#ffffff")
	ctx.Set("textAlign", "center")
	ctx.Call("fillText", g.Score, cw/2, 50)

	ctx.Set("font", "11px 'Space Mono', monospace")
	ctx.Set("fillStyle", "rgba(255,255,255,0.4)")
	ctx.Call("fillText", "SCORE", cw/2, 66)
}

func (g *Game) drawGrid() {
	ctx := g.Ctx
	ox, oy := g.gridOrigin()
	size := float64(GridSize) * CellSize

	// grid shadow
	ctx.Set("shadowColor", "rgba(0,0,0,0.5)")
	ctx.Set("shadowBlur", 20)
	ctx.Set("fillStyle", "rgba(255,255,255,0.04)")
	roundRect(ctx, ox-GridPad, oy-GridPad, size+GridPad*2, size+GridPad*2, 20)
	ctx.Call("fill")
	ctx.Set("shadowBlur", 0)

	// cells
	for row := 0; row < GridSize; row++ {
		for col := 0; col < GridSize; col++ {
			x := ox + float64(col)*CellSize
			y := oy + float64(row)*CellSize
			if g.Grid[row][col].Filled {
				drawCell(ctx, x, y, CellSize-3, g.Grid[row][col].Color, 1.0)
			} else {
				// empty cell
				ctx.Set("fillStyle", "rgba(255,255,255,0.05)")
				roundRect(ctx, x+1, y+1, CellSize-5, CellSize-5, 8)
				ctx.Call("fill")
			}
		}
	}

	// hover highlight when dragging
	if g.Dragging >= 0 && g.Pieces[g.Dragging] != nil {
		p := g.Pieces[g.Dragging]
		pw := g.pieceWidth(p)
		ph := g.pieceHeight(p)
		gx, gy := g.pixelToGrid(g.MouseX-float64(pw)*CellSize/2, g.MouseY-float64(ph)*CellSize/2)
		if g.canPlace(p, gx, gy) {
			for row := range p.Cells {
				for col := range p.Cells[row] {
					if p.Cells[row][col] {
						x := ox + float64(gx+col)*CellSize
						y := oy + float64(gy+row)*CellSize
						ctx.Set("fillStyle", p.Color)
						ctx.Set("globalAlpha", 0.35)
						roundRect(ctx, x+1, y+1, CellSize-5, CellSize-5, 8)
						ctx.Call("fill")
						ctx.Set("globalAlpha", 1.0)
					}
				}
			}
		}
	}
}

func (g *Game) drawPieces() {
	ctx := g.Ctx
	cw := g.Canvas.Get("width").Float()
	ch := g.Canvas.Get("height").Float()

	// bottom panel bg
	ctx.Set("fillStyle", "rgba(255,255,255,0.04)")
	roundRect(ctx, 8, ch-PieceArea-8, cw-16, PieceArea, 20)
	ctx.Call("fill")

	for i, p := range g.Pieces {
		if p == nil || i == g.Dragging {
			continue
		}
		cx, cy := g.pieceSlotPos(i)
		g.drawPieceAt(p, cx, cy, 1.0)
	}
}

func (g *Game) drawPieceAt(p *Piece, cx, cy, alpha float64) {
	pw := float64(g.pieceWidth(p))
	ph := float64(g.pieceHeight(p))
	startX := cx - pw*CellSize/2
	startY := cy - ph*CellSize/2

	for row := range p.Cells {
		for col := range p.Cells[row] {
			if p.Cells[row][col] {
				x := startX + float64(col)*CellSize
				y := startY + float64(row)*CellSize
				drawCell(g.Ctx, x, y, CellSize-3, p.Color, alpha)
			}
		}
	}
}

func (g *Game) drawDragging() {
	if g.Dragging < 0 || g.Pieces[g.Dragging] == nil {
		return
	}
	p := g.Pieces[g.Dragging]
	pw := float64(g.pieceWidth(p))
	ph := float64(g.pieceHeight(p))
	g.drawPieceAt(p, g.MouseX-pw*CellSize/2+pw*CellSize/2, g.MouseY-ph*CellSize/2+ph*CellSize/2, 0.92)
}

func (g *Game) drawAnimations() {
	remaining := g.Animating[:0]
	for _, a := range g.Animating {
		if a.Alpha <= 0 {
			continue
		}
		ox, oy := g.gridOrigin()
		x := ox + float64(a.X)*CellSize
		y := oy + float64(a.Y)*CellSize
		drawCell(g.Ctx, x, y, CellSize-3, a.Color, a.Alpha)
		a.Alpha -= 0.07
		remaining = append(remaining, a)
	}
	g.Animating = remaining
}

func (g *Game) drawGameOver() {
	ctx := g.Ctx
	cw := g.Canvas.Get("width").Float()
	ch := g.Canvas.Get("height").Float()

	ctx.Set("fillStyle", "rgba(15,14,23,0.88)")
	ctx.Call("fillRect", 0, 0, cw, ch)

	ctx.Set("font", "bold 38px 'Space Mono', monospace")
	ctx.Set("fillStyle", "#FF4D6D")
	ctx.Set("textAlign", "center")
	ctx.Call("fillText", "GAME OVER", cw/2, ch/2-30)

	ctx.Set("font", "18px 'Space Mono', monospace")
	ctx.Set("fillStyle", "rgba(255,255,255,0.7)")
	ctx.Call("fillText", "Score: "+itoa(g.Score), cw/2, ch/2+10)

	ctx.Set("fillStyle", "rgba(255,255,255,0.15)")
	roundRect(ctx, cw/2-80, ch/2+35, 160, 44, 12)
	ctx.Call("fill")
	ctx.Set("font", "14px 'Space Mono', monospace")
	ctx.Set("fillStyle", "#ffffff")
	ctx.Call("fillText", "TAP TO RESTART", cw/2, ch/2+62)
}

func drawCell(ctx js.Value, x, y, size float64, color string, alpha float64) {
	ctx.Set("globalAlpha", alpha)
	// shadow
	ctx.Set("shadowColor", color)
	ctx.Set("shadowBlur", 10)
	ctx.Set("fillStyle", color)
	roundRect(ctx, x+1, y+1, size-2, size-2, 9)
	ctx.Call("fill")
	// highlight
	ctx.Set("shadowBlur", 0)
	ctx.Set("fillStyle", "rgba(255,255,255,0.18)")
	roundRect(ctx, x+3, y+3, size-10, 6, 4)
	ctx.Call("fill")
	ctx.Set("globalAlpha", 1.0)
}

func roundRect(ctx js.Value, x, y, w, h, r float64) {
	ctx.Call("beginPath")
	ctx.Call("moveTo", x+r, y)
	ctx.Call("lineTo", x+w-r, y)
	ctx.Call("quadraticCurveTo", x+w, y, x+w, y+r)
	ctx.Call("lineTo", x+w, y+h-r)
	ctx.Call("quadraticCurveTo", x+w, y+h, x+w-r, y+h)
	ctx.Call("lineTo", x+r, y+h)
	ctx.Call("quadraticCurveTo", x, y+h, x, y+h-r)
	ctx.Call("lineTo", x, y+r)
	ctx.Call("quadraticCurveTo", x, y, x+r, y)
	ctx.Call("closePath")
}

// ---- Input ----

func (g *Game) onMouseDown(_ js.Value, args []js.Value) interface{} {
	if g.GameOver {
		g.restart()
		return nil
	}
	e := args[0]
	cx, cy := g.canvasXY()
	mx := e.Get("clientX").Float() - cx
	my := e.Get("clientY").Float() - cy
	g.MouseX = mx
	g.MouseY = my

	ch := g.Canvas.Get("height").Float()
	// check if clicking on a piece slot
	if my > ch-PieceArea-8 {
		for i, p := range g.Pieces {
			if p == nil {
				continue
			}
			sx, sy := g.pieceSlotPos(i)
			pw := float64(g.pieceWidth(p)) * CellSize / 2
			ph := float64(g.pieceHeight(p)) * CellSize / 2
			if mx >= sx-pw && mx <= sx+pw && my >= sy-ph && my <= sy+ph {
				g.Dragging = i
				break
			}
		}
	}
	return nil
}

func (g *Game) onMouseMove(_ js.Value, args []js.Value) interface{} {
	e := args[0]
	cx, cy := g.canvasXY()
	g.MouseX = e.Get("clientX").Float() - cx
	g.MouseY = e.Get("clientY").Float() - cy
	return nil
}

func (g *Game) onMouseUp(_ js.Value, _ []js.Value) interface{} {
	if g.Dragging < 0 {
		return nil
	}
	p := g.Pieces[g.Dragging]
	if p != nil {
		pw := float64(g.pieceWidth(p))
		ph := float64(g.pieceHeight(p))
		gx, gy := g.pixelToGrid(g.MouseX-pw*CellSize/2, g.MouseY-ph*CellSize/2)
		if g.canPlace(p, gx, gy) {
			g.placePiece(p, gx, gy)
			g.Pieces[g.Dragging] = nil
			cleared := g.clearLines()
			if cleared > 0 {
				g.Score += cleared * cleared * 100
			}
			// count placed cells for score
			for row := range p.Cells {
				for col := range p.Cells[row] {
					if p.Cells[row][col] {
						g.Score += 10
					}
				}
			}
			// refill if all used
			allNil := true
			for _, pp := range g.Pieces {
				if pp != nil {
					allNil = false
					break
				}
			}
			if allNil {
				g.generatePieces()
			}
			if g.checkGameOver() {
				g.GameOver = true
			}
		}
	}
	g.Dragging = -1
	return nil
}

func (g *Game) onTouchStart(_ js.Value, args []js.Value) interface{} {
	e := args[0]
	e.Call("preventDefault")
	touches := e.Get("changedTouches")
	if touches.Length() == 0 {
		return nil
	}
	t := touches.Index(0)
	cx, cy := g.canvasXY()
	mx := t.Get("clientX").Float() - cx
	my := t.Get("clientY").Float() - cy

	if g.GameOver {
		g.restart()
		return nil
	}

	g.MouseX = mx
	g.MouseY = my
	ch := g.Canvas.Get("height").Float()
	if my > ch-PieceArea-8 {
		for i, p := range g.Pieces {
			if p == nil {
				continue
			}
			sx, sy := g.pieceSlotPos(i)
			pw := float64(g.pieceWidth(p)) * CellSize / 2
			ph := float64(g.pieceHeight(p)) * CellSize / 2
			if mx >= sx-pw && mx <= sx+pw && my >= sy-ph && my <= sy+ph {
				g.Dragging = i
				break
			}
		}
	}
	return nil
}

func (g *Game) onTouchMove(_ js.Value, args []js.Value) interface{} {
	e := args[0]
	e.Call("preventDefault")
	touches := e.Get("changedTouches")
	if touches.Length() == 0 {
		return nil
	}
	t := touches.Index(0)
	cx, cy := g.canvasXY()
	g.MouseX = t.Get("clientX").Float() - cx
	g.MouseY = t.Get("clientY").Float() - cy
	return nil
}

func (g *Game) onTouchEnd(_ js.Value, args []js.Value) interface{} {
	e := args[0]
	e.Call("preventDefault")
	g.onMouseUp(js.Value{}, nil)
	return nil
}

func (g *Game) restart() {
	g.Grid = [GridSize][GridSize]Cell{}
	g.Score = 0
	g.Pieces = [3]*Piece{}
	g.Dragging = -1
	g.GameOver = false
	g.Animating = nil
	g.generatePieces()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 20)
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func main() {
	game := NewGame()

	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "gameCanvas")
	game.Canvas = canvas
	game.Ctx = canvas.Call("getContext", "2d")

	// resize canvas
	resizeCanvas := func() {
		w := js.Global().Get("innerWidth").Float()
		h := js.Global().Get("innerHeight").Float()
		if w > 480 {
			w = 480
		}
		canvas.Set("width", w)
		canvas.Set("height", h)
	}
	resizeCanvas()

	// event listeners
	canvas.Call("addEventListener", "mousedown", js.FuncOf(game.onMouseDown))
	canvas.Call("addEventListener", "mousemove", js.FuncOf(game.onMouseMove))
	canvas.Call("addEventListener", "mouseup", js.FuncOf(game.onMouseUp))
	canvas.Call("addEventListener", "touchstart", js.FuncOf(game.onTouchStart), map[string]interface{}{"passive": false})
	canvas.Call("addEventListener", "touchmove", js.FuncOf(game.onTouchMove), map[string]interface{}{"passive": false})
	canvas.Call("addEventListener", "touchend", js.FuncOf(game.onTouchEnd), map[string]interface{}{"passive": false})
	js.Global().Call("addEventListener", "resize", js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		resizeCanvas()
		return nil
	}))

	// game loop
	var loop js.Func
	loop = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		game.render(js.Value{}, nil)
		js.Global().Call("requestAnimationFrame", loop)
		return nil
	})
	js.Global().Call("requestAnimationFrame", loop)

	select {}
}
