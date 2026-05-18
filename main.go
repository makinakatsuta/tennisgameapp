package main

import (
	"fmt"
	"image/color"
	"log"
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

type Game struct {
	logic *game.GameLogic
	audio *audio.Engine
}

func NewGame() *Game {
	l := game.NewGameLogic()
	ae := audio.NewEngine()

	l.OnServe = func(isP1 bool) { ae.PlayServe(isP1) }
	l.OnHit = func(isP1 bool) { ae.PlayHit(isP1) }
	l.OnBounce = func(x, y float64) { ae.PlayBounce(x, y) }
	l.OnVoice = func(text string) { ae.PlayVoice(text) }

	l.OnVoice("試合開始。")

	return &Game{
		logic: l,
		audio: ae,
	}
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

	if ebiten.IsKeyPressed(ebiten.KeyLeft) { g.logic.MovePlayer(1, "left") }
	if ebiten.IsKeyPressed(ebiten.KeyRight) { g.logic.MovePlayer(1, "right") }
	if ebiten.IsKeyPressed(ebiten.KeyUp) { g.logic.MovePlayer(1, "up") }
	if ebiten.IsKeyPressed(ebiten.KeyDown) { g.logic.MovePlayer(1, "down") }

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
			g.audio.PlaySwing()
		}
	}

	g.logic.Update()
	b := g.logic.Ball
	g.audio.UpdateBallSound(b.Pos.X, b.Pos.Y, b.Pos.Z, b.Vel.X, b.Vel.Y, b.Vel.Z)

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{0x11, 0x11, 0x11, 0xff})

	cx, cy := float32(screenWidth/2), float32(screenHeight/2)
	scale := float32(0.5) // Scale down to fit court

	// Draw Court (Green)
	vector.DrawFilledRect(screen, cx-305*scale, cy-670*scale, 610*scale, 1340*scale, color.RGBA{0x2e, 0x7d, 0x32, 0xff}, false)
	// Net
	vector.StrokeLine(screen, cx-305*scale, cy, cx+305*scale, cy, 4, color.White, false)
	// Outer Border
	vector.StrokeRect(screen, cx-305*scale, cy-670*scale, 610*scale, 1340*scale, 8, color.White, false)

	// Draw Players
	vector.DrawFilledCircle(screen, cx+float32(g.logic.P1.X)*scale, cy+float32(g.logic.P1.Y)*scale, 20*scale, color.RGBA{0x34, 0x98, 0xdb, 0xff}, false)
	vector.DrawFilledCircle(screen, cx+float32(g.logic.P2.X)*scale, cy+float32(g.logic.P2.Y)*scale, 20*scale, color.RGBA{0xe7, 0x4c, 0x3c, 0xff}, false)

	// Draw Ball (with height offset)
	ballZ := float32(g.logic.Ball.Pos.Z)
	vector.DrawFilledCircle(screen, cx+float32(g.logic.Ball.Pos.X)*scale, cy+float32(g.logic.Ball.Pos.Y)*scale-ballZ*scale, 10*(1+ballZ/100)*scale, color.RGBA{0xff, 0xff, 0x00, 0xff}, false)

	// HUD
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Score: Player %d - Computer %d", g.logic.Score.P1, g.logic.Score.P2))
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	ebiten.SetWindowTitle("Blind Tennis (Go)")
	ebiten.SetWindowSize(600, 600) // Scaled down for desktop view

	g := NewGame()
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
