package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"tennis-go/internal/audio"
	"tennis-go/internal/game"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	screenWidth  = 800
	screenHeight = 800
)

type Ripple struct {
	X, Y float64 // Court coordinates
	Age  int     // Frames elapsed (30 frames max)
}

type TrailPoint struct {
	X, Y, Z float64
}

type Game struct {
	logic        *game.GameLogic
	audio        *audio.Engine
	ripples      []Ripple
	trail        []TrailPoint
	p1SwingTimer int
	p2SwingTimer int
}

func NewGame() *Game {
	l := game.NewGameLogic()
	ae := audio.NewEngine()

	g := &Game{
		logic: l,
		audio: ae,
	}

	l.OnServe = func(isP1 bool) {
		ae.PlayServe(isP1)
		if isP1 {
			g.p1SwingTimer = 15
		} else {
			g.p2SwingTimer = 15
		}
	}
	l.OnHit = func(isP1 bool) {
		ae.PlayHit(isP1)
		if isP1 {
			g.p1SwingTimer = 15
		} else {
			g.p2SwingTimer = 15
		}
	}
	l.OnBounce = func(x, y float64) {
		ae.PlayBounce(x, y)
		g.ripples = append(g.ripples, Ripple{X: x, Y: y, Age: 0})
	}
	l.OnVoice = func(text string) { ae.PlayVoice(text) }

	l.OnVoice("試合開始。")

	return g
}

func (g *Game) Update() error {
	// Serve Selection (1, 2, 3 keys)
	if g.logic.State == game.StateServing && g.logic.Server == 1 {
		if inpututil.IsKeyJustPressed(ebiten.Key1) {
			g.logic.SelectedServeType = game.ServeSlice
			g.logic.OnVoice("スライスサーブ")
		} else if inpututil.IsKeyJustPressed(ebiten.Key2) {
			g.logic.SelectedServeType = game.ServeDrive
			g.logic.OnVoice("ドライブサーブ")
		} else if inpututil.IsKeyJustPressed(ebiten.Key3) {
			g.logic.SelectedServeType = game.ServeUnder
			g.logic.OnVoice("アンダーサーブ")
		}
	}

	// AI Logic
	if g.logic.State == game.StateServing {
		if g.logic.Server == 1 && g.logic.Ready.P1 && !g.logic.Ready.P2 {
			g.logic.Ready.P2 = true
			g.logic.OnVoice("はい")
		} else if g.logic.Server == 2 && !g.logic.Ready.P2 {
			g.logic.Ready.P2 = true
			g.logic.OnVoice("いきます")
		} else if g.logic.Server == 2 && g.logic.Ready.P1 && g.logic.Ready.P2 {
			g.logic.Serve(2)
		}
	}

	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.logic.MovePlayer(1, "left")
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.logic.MovePlayer(1, "right")
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.logic.MovePlayer(1, "up")
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.logic.MovePlayer(1, "down")
	}

	// AI Player 2 Movement and Swing
	if g.logic.State == game.StateRally {
		// Simple tracking AI
		targetX := g.logic.Ball.Pos.X
		if g.logic.P2.X < targetX-5 {
			g.logic.MovePlayer(2, "right")
		} else if g.logic.P2.X > targetX+5 {
			g.logic.MovePlayer(2, "left")
		}

		// Attempt to swing when ball is close
		distY := g.logic.Ball.Pos.Y - g.logic.P2.Y
		if distY > -100 && distY < 0 && g.logic.Ball.Vel.Y < 0 {
			g.logic.Swing(2)
			g.p2SwingTimer = 15 // Trigger visual swing trail for P2 (CPU)
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if g.logic.State == game.StateServing {
			if g.logic.Server == 1 && !g.logic.Ready.P1 {
				g.logic.Ready.P1 = true
				g.logic.OnVoice("いきます")
			} else if g.logic.Server == 1 && g.logic.Ready.P2 {
				g.audio.PlaySwing()
				g.logic.Serve(1)
			} else if g.logic.Server == 2 && !g.logic.Ready.P1 {
				g.logic.Ready.P1 = true
				g.logic.OnVoice("はい")
			} else {
				g.audio.PlaySwing()
			}
		} else {
			if g.logic.State == game.StateRally {
				g.logic.Swing(1)
			}
			g.p1SwingTimer = 15 // Trigger visual swing trail for P1 (User)
			g.audio.PlaySwing()
		}
	}

	g.logic.Update()
	b := g.logic.Ball
	g.audio.UpdateBallSound(b.Pos.X, b.Pos.Y, b.Pos.Z, b.Vel.X, b.Vel.Y, b.Vel.Z)

	// Update ball flight trail
	if g.logic.State == game.StateRally {
		g.trail = append(g.trail, TrailPoint{
			X: b.Pos.X,
			Y: b.Pos.Y,
			Z: b.Pos.Z,
		})
		if len(g.trail) > 15 {
			g.trail = g.trail[1:]
		}
	} else {
		g.trail = nil
	}

	// Update bounce ripples
	activeRipples := []Ripple{}
	for _, r := range g.ripples {
		r.Age++
		if r.Age < 30 {
			activeRipples = append(activeRipples, r)
		}
	}
	g.ripples = activeRipples

	// Update swing timers
	if g.p1SwingTimer > 0 {
		g.p1SwingTimer--
	}
	if g.p2SwingTimer > 0 {
		g.p2SwingTimer--
	}

	return nil
}

// project maps 3D court coordinates to 2D screen coordinates with pseudo-3D perspective
func (g *Game) project(x, y, z float64) (float32, float32, float32) {
	cx := 400.0
	horizonY := 300.0
	cDepth := 151800.0
	f := 380.0

	dy := 1000.0 - y
	if dy < 50 {
		dy = 50
	}

	scale := f / dy
	sx := cx + x*scale
	sy := horizonY + cDepth/dy - z*scale

	return float32(sx), float32(sy), float32(scale)
}

func (g *Game) drawPlayer(screen *ebiten.Image, x, y float64, isP1 bool) {
	// Project player ground position (Z = 0)
	px, py, pScale := g.project(x, y, 0)

	// Project player top position (Z = 60) for character hologram body
	ptx, pty, _ := g.project(x, y, 60)

	var primaryCol, secondaryCol, glowCol color.Color
	var swingTimer int
	if isP1 {
		// Player 1 (User): Neon Cyan
		primaryCol = color.RGBA{0, 180, 255, 0xff}
		secondaryCol = color.RGBA{180, 230, 255, 0xff}
		glowCol = color.RGBA{0, 210, 255, 0x22}
		swingTimer = g.p1SwingTimer
	} else {
		// Player 2 (Computer): Neon Crimson
		primaryCol = color.RGBA{220, 0, 70, 0xff}
		secondaryCol = color.RGBA{255, 150, 180, 0xff}
		glowCol = color.RGBA{255, 0, 85, 0x22}
		swingTimer = g.p2SwingTimer
	}

	// 1. Draw footprint ring (ground placement marker)
	vector.StrokeCircle(screen, px, py, 20*pScale, 2.0, primaryCol, true)
	vector.DrawFilledCircle(screen, px, py, 20*pScale, glowCol, true)

	// 2. Draw racket reach/swing range visualizer
	if isP1 {
		// For Player 1, highlight in green if the ball is within hit range!
		dx := g.logic.Ball.Pos.X - g.logic.P1.X
		dy := g.logic.Ball.Pos.Y - g.logic.P1.Y
		dist := math.Sqrt(dx*dx + dy*dy)
		inRange := dist < 85 && g.logic.Ball.Pos.Z < 180

		rangeCol := color.RGBA{0, 210, 255, 0x15} // Default cyan glow
		if inRange {
			rangeCol = color.RGBA{170, 255, 0, 0x44} // Green glowing cue
			vector.StrokeCircle(screen, px, py, 85*pScale, 2.5, color.RGBA{170, 255, 0, 0xbb}, true)
		} else {
			vector.StrokeCircle(screen, px, py, 85*pScale, 1.2, color.RGBA{0, 210, 255, 0x44}, true)
		}
		vector.DrawFilledCircle(screen, px, py, 85*pScale, rangeCol, true)
	} else {
		// Computer reach circle (subtle crimson)
		vector.StrokeCircle(screen, px, py, 85*pScale, 1.0, color.RGBA{255, 0, 85, 0x22}, true)
	}

	// 3. Draw active swing wave animation
	if swingTimer > 0 {
		progress := float64(15-swingTimer) / 15.0
		rStart := 20.0
		rEnd := 90.0
		rCurrent := (rStart + progress*(rEnd-rStart)) * float64(pScale)
		alpha := uint8((1.0 - progress) * 220)

		var sCol color.Color
		if isP1 {
			sCol = color.RGBA{0, 210, 255, alpha}
		} else {
			sCol = color.RGBA{255, 0, 85, alpha}
		}
		vector.StrokeCircle(screen, px, py, float32(rCurrent), 3.5, sCol, true)
	}

	// 4. Draw character hologram cylinder (glowing lines)
	vector.StrokeLine(screen, px, py, ptx, pty, 7*pScale, primaryCol, true)
	vector.StrokeLine(screen, px, py, ptx, pty, 2.5*pScale, secondaryCol, true)

	// 5. Draw floating head
	vector.DrawFilledCircle(screen, ptx, pty, 11*pScale, primaryCol, true)
	vector.DrawFilledCircle(screen, ptx, pty, 7*pScale, secondaryCol, true)
}

func (g *Game) drawNet(screen *ebiten.Image) {
	ntlX, ntlY, _ := g.project(-305, 0, 80)
	ntrX, ntrY, _ := g.project(305, 0, 80)
	nblX, nblY, _ := g.project(-305, 0, 0)
	nbrX, nbrY, _ := g.project(305, 0, 0)

	netColor := color.RGBA{0, 229, 255, 0x33} // Soft Cyan Mesh
	postColor := color.RGBA{0xdd, 0xdd, 0xdd, 0xff}
	topBandColor := color.White

	// Draw vertical posts
	vector.StrokeLine(screen, nblX, nblY, ntlX, ntlY, 5.0, postColor, true)
	vector.StrokeLine(screen, nbrX, nbrY, ntrX, ntrY, 5.0, postColor, true)

	// Draw horizontal grid lines
	for h := 1; h <= 4; h++ {
		hz := float64(h) * 16.0
		lx, ly, _ := g.project(-305, 0, hz)
		rx, ry, _ := g.project(305, 0, hz)
		vector.StrokeLine(screen, lx, ly, rx, ry, 1.0, netColor, true)
	}

	// Draw vertical grid lines
	for i := 1; i < 15; i++ {
		t := float64(i) / 15.0
		x := -305.0 + t*610.0
		bx, by, _ := g.project(x, 0, 0)
		tx, ty, _ := g.project(x, 0, 80)
		vector.StrokeLine(screen, bx, by, tx, ty, 1.0, netColor, true)
	}

	// Draw top white net tape
	vector.StrokeLine(screen, ntlX, ntlY, ntrX, ntrY, 4.0, topBandColor, true)
}

func (g *Game) drawBallShadow(screen *ebiten.Image) {
	b := g.logic.Ball
	shX, shY, shScale := g.project(b.Pos.X, b.Pos.Y, 0)
	shRadius := 10.0 * float64(shScale)

	// Drop shadow fades with height (Z)
	alphaVal := 180.0 - b.Pos.Z*1.2
	if alphaVal < 25 {
		alphaVal = 25
	}
	if alphaVal > 180 {
		alphaVal = 180
	}

	// Shadow representation (semi-transparent dark ellipse)
	vector.DrawFilledCircle(screen, shX, shY, float32(shRadius), color.RGBA{0, 0, 0, uint8(alphaVal)}, true)
	vector.StrokeCircle(screen, shX, shY, float32(shRadius), 1.2, color.RGBA{0, 0, 0, uint8(alphaVal / 2)}, true)
}

func (g *Game) drawBallTrail(screen *ebiten.Image) {
	if len(g.trail) == 0 {
		return
	}
	for i, pt := range g.trail {
		tx, ty, tScale := g.project(pt.X, pt.Y, pt.Z)
		t := float64(i) / float64(len(g.trail))
		radius := float32(7.0 * float64(tScale) * (0.3 + 0.7*t))
		alpha := uint8(20.0 + 120.0*t)

		// Glowing neon yellow-green trail
		vector.DrawFilledCircle(screen, tx, ty, radius, color.RGBA{0xbd, 0xff, 0x08, alpha}, true)
	}
}

func (g *Game) drawBall(screen *ebiten.Image) {
	b := g.logic.Ball
	bx, by, bScale := g.project(b.Pos.X, b.Pos.Y, b.Pos.Z)
	radius := 10.0 * float64(bScale)

	// Outer glow ring
	vector.StrokeCircle(screen, bx, by, float32(radius*1.3), 2.0, color.RGBA{0xbd, 0xff, 0x08, 0x88}, true)
	// Ball body
	vector.DrawFilledCircle(screen, bx, by, float32(radius), color.RGBA{0xbd, 0xff, 0x08, 0xff}, true)
	// Glowing core
	vector.DrawFilledCircle(screen, bx, by, float32(radius*0.5), color.White, true)
}

func (g *Game) drawRipples(screen *ebiten.Image) {
	for _, r := range g.ripples {
		rx, ry, rScale := g.project(r.X, r.Y, 0)
		progress := float64(r.Age) / 30.0
		radius := progress * 65.0 * float64(rScale)
		alpha := uint8((1.0 - progress) * 160)

		// Neon green expanding rings
		vector.StrokeCircle(screen, rx, ry, float32(radius), 2.0, color.RGBA{170, 255, 0, alpha}, true)
		vector.StrokeCircle(screen, rx, ry, float32(radius * 0.8), 1.0, color.RGBA{0, 229, 255, alpha / 2}, true)
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	// 1. Background fill (Cyberpunk deep space violet)
	screen.Fill(color.RGBA{0x08, 0x06, 0x11, 0xff})

	// 2. Draw Sky Grid & Horizon
	for i := -10; i <= 10; i++ {
		xVal := float32(400 + i*90)
		vector.StrokeLine(screen, 400, 300, xVal, 0, 1.0, color.RGBA{0x3d, 0x18, 0x6e, 0x33}, true)
	}
	for h := 0; h < 7; h++ {
		sy := float32(300 - 300.0*math.Pow(float64(h)/7.0, 1.6))
		vector.StrokeLine(screen, 0, sy, 800, sy, 1.0, color.RGBA{0x3d, 0x18, 0x6e, 0x33}, true)
	}

	// Retro starry background
	stars := []struct{ X, Y float32; Size float32; Alpha uint8 }{
		{120, 80, 1.5, 120}, {250, 140, 1.0, 80}, {380, 50, 2.0, 180},
		{540, 120, 1.0, 90}, {680, 90, 1.5, 150}, {730, 160, 2.0, 100},
		{180, 210, 1.0, 70}, {620, 200, 1.0, 60}, {310, 100, 1.5, 110},
	}
	for _, star := range stars {
		vector.DrawFilledCircle(screen, star.X, star.Y, star.Size, color.RGBA{0x00, 0xe5, 0xff, star.Alpha}, true)
	}

	// Glowing magenta horizon line
	vector.StrokeLine(screen, 0, 300, 800, 300, 2.5, color.RGBA{0xff, 0x00, 0x77, 0xaa}, true)

	// 3. Draw Court surface scanlines (from Y = -670 (sy = 391) to Y = 670 (sy = 760))
	topY := 391.0
	bottomY := 760.0
	cx := 400.0
	cDepth := 151800.0
	f := 380.0
	horizonY := 300.0

	for sy := int(topY); sy <= int(bottomY); sy++ {
		dy := cDepth / (float64(sy) - horizonY)
		scale := f / dy
		sxLeft := cx - 305.0*scale
		sxRight := cx + 305.0*scale

		// Row color gradient: Deep dark violet-blue to rich midnight indigo
		t := float64(sy-int(topY)) / float64(int(bottomY)-int(topY))
		r := uint8(0x0a + t*0x14)
		g := uint8(0x0e + t*0x0d)
		b := uint8(0x22 + t*0x28)

		vector.StrokeLine(screen, float32(sxLeft), float32(sy), float32(sxRight), float32(sy), 1.2, color.RGBA{r, g, b, 0xff}, false)
	}

	// 4. Draw Court lines & service grid on top of scanlines
	xTL, yTL, _ := g.project(-305, -670, 0)
	xTR, yTR, _ := g.project(305, -670, 0)
	xBL, yBL, _ := g.project(-305, 670, 0)
	xBR, yBR, _ := g.project(305, 670, 0)

	courtLineColor := color.RGBA{0x00, 0xe5, 0xff, 0x99}      // Neon Cyan
	courtLineColorOuter := color.RGBA{0x00, 0xe5, 0xff, 0xee} // Bright Neon Cyan

	// Outer borders
	vector.StrokeLine(screen, xTL, yTL, xTR, yTR, 2.0, courtLineColorOuter, true) // Far baseline
	vector.StrokeLine(screen, xBL, yBL, xBR, yBR, 4.0, courtLineColorOuter, true) // Near baseline (thicker)
	vector.StrokeLine(screen, xTL, yTL, xBL, yBL, 3.0, courtLineColorOuter, true) // Left sideline
	vector.StrokeLine(screen, xTR, yTR, xBR, yBR, 3.0, courtLineColorOuter, true) // Right sideline

	// Singles sidelines
	xS1L, yS1L, _ := g.project(-225, -670, 0)
	xS2L, yS2L, _ := g.project(-225, 670, 0)
	vector.StrokeLine(screen, xS1L, yS1L, xS2L, yS2L, 1.5, courtLineColor, true)

	xS1R, yS1R, _ := g.project(225, -670, 0)
	xS2R, yS2R, _ := g.project(225, 670, 0)
	vector.StrokeLine(screen, xS1R, yS1R, xS2R, yS2R, 1.5, courtLineColor, true)

	// Service lines (Y = -320 and Y = 320)
	xSerTL, ySerTL, _ := g.project(-225, -320, 0)
	xSerTR, ySerTR, _ := g.project(225, -320, 0)
	vector.StrokeLine(screen, xSerTL, ySerTL, xSerTR, ySerTR, 2.0, courtLineColor, true)

	xSerBL, ySerBL, _ := g.project(-225, 320, 0)
	xSerBR, ySerBR, _ := g.project(225, 320, 0)
	vector.StrokeLine(screen, xSerBL, ySerBL, xSerBR, ySerBR, 2.5, courtLineColor, true)

	// Center service line (X = 0, from Y = -320 to Y = 320)
	xCenterT, yCenterT, _ := g.project(0, -320, 0)
	xCenterB, yCenterB, _ := g.project(0, 320, 0)
	vector.StrokeLine(screen, xCenterT, yCenterT, xCenterB, yCenterB, 1.8, courtLineColor, true)

	// Inner grid lines (subtle, purple/magenta)
	gridColor := color.RGBA{0x8b, 0x00, 0xff, 0x22}
	for gy := -600.0; gy <= 600.0; gy += 150.0 {
		if math.Abs(gy) < 50 {
			continue // Net is near Y=0
		}
		x1, y1, _ := g.project(-305, gy, 0)
		x2, y2, _ := g.project(305, gy, 0)
		vector.StrokeLine(screen, x1, y1, x2, y2, 1.0, gridColor, true)
	}
	for _, gx := range []float64{-112.5, 112.5} {
		x1, y1, _ := g.project(gx, -670, 0)
		x2, y2, _ := g.project(gx, 670, 0)
		vector.StrokeLine(screen, x1, y1, x2, y2, 1.0, gridColor, true)
	}

	// 5. Draw bounce ripples
	g.drawRipples(screen)

	// 6. Draw players and ball in correct depth sorting relative to net
	// Player 2 is always behind the net (Y < 0)
	g.drawPlayer(screen, g.logic.P2.X, g.logic.P2.Y, false)

	ballY := g.logic.Ball.Pos.Y
	if ballY < 0 {
		// Ball is in far court (behind net)
		g.drawBallShadow(screen)
		g.drawBallTrail(screen)
		g.drawBall(screen)

		g.drawNet(screen)

		// Draw Player 1 (User, close)
		g.drawPlayer(screen, g.logic.P1.X, g.logic.P1.Y, true)
	} else {
		// Ball is in near court (in front of net)
		g.drawNet(screen)

		// Draw Player 1 (User, close)
		g.drawPlayer(screen, g.logic.P1.X, g.logic.P1.Y, true)

		g.drawBallShadow(screen)
		g.drawBallTrail(screen)
		g.drawBall(screen)
	}

	// 7. Draw HUD (scoreboards, panels, messages)
	// Translucent top HUD panel
	vector.DrawFilledRect(screen, 0, 0, 800, 100, color.RGBA{0x06, 0x04, 0x0c, 0xd8}, true)
	vector.StrokeLine(screen, 0, 100, 800, 100, 2, color.RGBA{0x44, 0x33, 0x66, 0xaa}, true)

	// Score text labels
	ebitenutil.DebugPrintAt(screen, "PLAYER 1 (YOU)", 40, 15)
	ebitenutil.DebugPrintAt(screen, "COMPUTER", 680, 15)

	p1PointsStr := getPointScoreStr(g.logic.PointScore.P1)
	p2PointsStr := getPointScoreStr(g.logic.PointScore.P2)

	// Large 7-segment Points scoreboard
	draw7SegmentString(screen, 40, 35, p1PointsStr, 18, color.RGBA{0, 210, 255, 0xff})
	draw7SegmentString(screen, 680, 35, p2PointsStr, 18, color.RGBA{255, 0, 85, 0xff})

	// Games won scoreboard
	ebitenutil.DebugPrintAt(screen, "GAMES", 130, 15)
	ebitenutil.DebugPrintAt(screen, "GAMES", 610, 15)

	draw7SegmentString(screen, 130, 35, fmt.Sprintf("%d", g.logic.Score.P1), 18, color.RGBA{255, 230, 0, 0xff})
	draw7SegmentString(screen, 610, 35, fmt.Sprintf("%d", g.logic.Score.P2), 18, color.RGBA{255, 230, 0, 0xff})

	// Center HUD Status Panel
	vector.StrokeRect(screen, 210, 12, 380, 76, 1.5, color.RGBA{0x44, 0x33, 0x66, 0xdd}, true)
	vector.DrawFilledRect(screen, 211, 13, 378, 74, color.RGBA{0x09, 0x07, 0x10, 0xff}, true)

	// Render status messages in Center HUD Panel
	if g.logic.State == game.StateServing {
		if g.logic.Server == 1 {
			ebitenutil.DebugPrintAt(screen, "★ YOUR SERVE ★", 350, 18)
			ebitenutil.DebugPrintAt(screen, "[1] Slice   [2] Drive   [3] Under", 280, 38)

			// Highlight selected serve type
			switch g.logic.SelectedServeType {
			case game.ServeSlice:
				vector.StrokeRect(screen, 275, 36, 75, 18, 1.2, color.RGBA{0, 210, 255, 0xff}, true)
			case game.ServeDrive:
				vector.StrokeRect(screen, 358, 36, 75, 18, 1.2, color.RGBA{0, 210, 255, 0xff}, true)
			case game.ServeUnder:
				vector.StrokeRect(screen, 442, 36, 75, 18, 1.2, color.RGBA{0, 210, 255, 0xff}, true)
			}

			if !g.logic.Ready.P1 {
				ebitenutil.DebugPrintAt(screen, "Press SPACE to Call 'Ready' (いきます)", 285, 58)
			} else if g.logic.Ready.P1 && !g.logic.Ready.P2 {
				ebitenutil.DebugPrintAt(screen, "Waiting for opponent response...", 295, 58)
			} else if g.logic.Ready.P1 && g.logic.Ready.P2 {
				ebitenutil.DebugPrintAt(screen, "READY! Press SPACE to serve the ball", 282, 58)
			}
		} else {
			ebitenutil.DebugPrintAt(screen, "★ COMPUTER SERVE ★", 335, 18)

			if !g.logic.Ready.P1 {
				ebitenutil.DebugPrintAt(screen, "CPU is ready. Press SPACE to Call 'Ready' (はい)", 255, 45)
			} else {
				ebitenutil.DebugPrintAt(screen, "Awaiting CPU serve...", 330, 45)
			}
		}
	} else if g.logic.State == game.StateRally {
		ebitenutil.DebugPrintAt(screen, "▲ RALLY ▲", 365, 18)
		ebitenutil.DebugPrintAt(screen, "Move: Arrow Keys  |  Swing: Space", 288, 38)

		bouncesText := fmt.Sprintf("Bounces: %d / 3", g.logic.Ball.Bounces)
		ebitenutil.DebugPrintAt(screen, bouncesText, 350, 58)
	} else if g.logic.State == game.StateScored {
		ebitenutil.DebugPrintAt(screen, "✦ POINT SCORED! ✦", 335, 22)
		ebitenutil.DebugPrintAt(screen, "Preparing next serve...", 325, 48)
	}
}

func getPointScoreStr(pScore int) string {
	scoreLabels := []string{"0", "15", "30", "40"}
	if pScore >= 0 && pScore < len(scoreLabels) {
		return scoreLabels[pScore]
	}
	return "0"
}

func draw7Segment(screen *ebiten.Image, x, y float32, char rune, size float32, col color.Color) {
	w := size
	h := size * 1.8
	t := size * 0.15 // thickness

	var a, b, c, d, e, f, g bool
	switch char {
	case '0':
		a, b, c, d, e, f = true, true, true, true, true, true
	case '1':
		b, c = true, true
	case '2':
		a, b, g, e, d = true, true, true, true, true
	case '3':
		a, b, g, c, d = true, true, true, true, true
	case '4':
		f, g, b, c = true, true, true, true
	case '5':
		a, f, g, c, d = true, true, true, true, true
	case '6':
		a, f, g, e, c, d = true, true, true, true, true, true
	case '7':
		a, b, c = true, true, true
	case '8':
		a, b, c, d, e, f, g = true, true, true, true, true, true, true
	case '9':
		a, b, c, d, f, g = true, true, true, true, true, true
	case '-':
		g = true
	}

	drawSeg := func(x1, y1, x2, y2 float32) {
		vector.StrokeLine(screen, x1, y1, x2, y2, t, col, true)
	}

	if a {
		drawSeg(x+t, y, x+w-t, y)
	}
	if f {
		drawSeg(x, y+t, x, y+h/2-t)
	}
	if b {
		drawSeg(x+w, y+t, x+w, y+h/2-t)
	}
	if g {
		drawSeg(x+t, y+h/2, x+w-t, y+h/2)
	}
	if e {
		drawSeg(x, y+h/2+t, x, y+h-t)
	}
	if c {
		drawSeg(x+w, y+h/2+t, x+w, y+h-t)
	}
	if d {
		drawSeg(x+t, y+h, x+w-t, y+h)
	}
}

func draw7SegmentString(screen *ebiten.Image, x, y float32, text string, size float32, col color.Color) {
	currX := x
	spacing := size * 1.4
	for _, char := range text {
		draw7Segment(screen, currX, y, char, size, col)
		currX += spacing
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowTitle("Blind Tennis (Go)")
	ebiten.SetWindowSize(800, 800) // Beautiful native 800x800 resolution

	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
