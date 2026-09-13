package game

import (
	"fmt"
	"math"
	"math/rand"
	"time"
)

type GameState int

const (
	StateServing GameState = iota
	StateRally
	StateScored
)

const (
	ServeSlice = 1
	ServeDrive = 2
	ServeUnder = 3
)

type Vector3 struct {
	X, Y, Z float64
}

type Ball struct {
	Pos     Vector3
	Vel     Vector3
	Bounces int
}

type Player struct {
	X, Y float64
}

type GameLogic struct {
	State             GameState
	Score             struct{ P1, P2 int }
	PointScore        struct{ P1, P2 int }
	P1, P2            Player
	Ball              Ball
	Server            int // 1 or 2
	Ready             struct{ P1, P2 bool }
	SelectedServeType int // For the player
	AiServeType       int // For the CPU
	AiClumsy          bool
	RNG               *rand.Rand

	// Callbacks for Sound
	OnServe  func(isP1 bool)
	OnHit    func(isP1 bool)
	OnBounce func(x, y float64)
	OnVoice  func(text string)

	// Continuous Play
	ScoredTimer int
}

func NewGameLogic() *GameLogic {
	g := &GameLogic{
		State:             StateServing,
		Server:            1,
		RNG:               rand.New(rand.NewSource(time.Now().UnixNano())),
		P1:                Player{X: 0, Y: CourtLength/2 - 50},
		P2:                Player{X: 0, Y: -CourtLength/2 + 50},
		SelectedServeType: ServeSlice,
		OnServe:           func(isP1 bool) {},
		OnHit:             func(isP1 bool) {},
		OnBounce:          func(x, y float64) {},
		OnVoice:           func(text string) {},
	}
	g.ResetBall(1)
	return g
}

func (g *GameLogic) ResetBall(nextServer int) {
	yPos := float64(CourtLength/2 - 100)
	if nextServer == 2 {
		yPos = -CourtLength/2 + 100
	}
	g.Ball = Ball{
		Pos:     Vector3{X: 0, Y: yPos, Z: 50},
		Vel:     Vector3{X: 0, Y: 0, Z: 0},
		Bounces: 0,
	}
	g.Ready.P1 = false
	g.Ready.P2 = false
	g.State = StateServing
	g.Server = nextServer
	g.SelectedServeType = ServeSlice
	g.AiClumsy = g.RNG.Float64() < 0.4
	if nextServer == 2 {
		// Randomly select the CPU serve type (33% each)
		g.AiServeType = g.RNG.Intn(3) + 1
	}
}

func (g *GameLogic) Serve(playerId int) {
	if !g.Ready.P1 || !g.Ready.P2 {
		return
	}
	if g.OnServe != nil {
		g.OnServe(playerId == 1)
	}

	serveType := g.SelectedServeType
	if playerId == 2 {
		serveType = g.AiServeType
	}

	var vy, vz float64
	switch serveType {
	case ServeSlice:
		vy = 4.0
		vz = 4.0
	case ServeDrive:
		vy = 3.0
		vz = 6.0
	case ServeUnder:
		vy = 2.0
		vz = 9.0
	default:
		vy = 3.0
		vz = 6.0
	}

	if playerId == 1 {
		vy = -vy
	}
	g.Ball.Vel = Vector3{
		X: (g.RNG.Float64() - 0.5) * 3.5,
		Y: vy,
		Z: vz,
	}
	g.Ball.Bounces = 0
	g.State = StateRally
}

func (g *GameLogic) Swing(playerId int) {
	var p Player
	if playerId == 1 {
		p = g.P1
	} else {
		p = g.P2
	}

	dx := g.Ball.Pos.X - p.X
	dy := g.Ball.Pos.Y - p.Y
	dist := math.Sqrt(dx*dx + dy*dy)

	if dist < 85 && g.Ball.Pos.Z < 180 {
		if g.OnHit != nil {
			g.OnHit(playerId == 1)
		}

		angle := dx / 35
		vy := 3.2
		if playerId == 1 {
			vy = -3.2
		}
		g.Ball.Vel = Vector3{
			X: angle * 4,
			Y: vy,
			Z: 5,
		}
		g.Ball.Bounces = 0
		if g.State == StateServing {
			g.State = StateRally
		}
	}
}

func (g *GameLogic) MovePlayer(playerId int, direction string) {
	speed := 8.0

	if playerId == 1 {
		switch direction {
		case "left":
			g.P1.X = math.Max(-CourtWidth/2, g.P1.X-speed)
		case "right":
			g.P1.X = math.Min(CourtWidth/2, g.P1.X+speed)
		case "up":
			g.P1.Y = math.Max(0, g.P1.Y-speed)
		case "down":
			g.P1.Y = math.Min(CourtLength/2, g.P1.Y+speed)
		}
	} else {
		switch direction {
		case "left":
			g.P2.X = math.Max(-CourtWidth/2, g.P2.X-speed)
		case "right":
			g.P2.X = math.Min(CourtWidth/2, g.P2.X+speed)
		case "up":
			g.P2.Y = math.Max(-CourtLength/2, g.P2.Y-speed)
		case "down":
			g.P2.Y = math.Min(0, g.P2.Y+speed)
		}
	}
}

func (g *GameLogic) Update() {
	if g.State == StateScored {
		g.ScoredTimer--
		if g.ScoredTimer <= 0 {
			nextServer := 1
			if (g.Score.P1+g.Score.P2)%2 != 0 {
				nextServer = 2
			}
			g.ResetBall(nextServer)
		}
		return
	}

	if g.State == StateRally {
		g.Ball.Pos.X += g.Ball.Vel.X
		g.Ball.Pos.Y += g.Ball.Vel.Y
		g.Ball.Pos.Z += g.Ball.Vel.Z
		g.Ball.Vel.Z -= Gravity
		g.Ball.Vel.X *= AirResistance
		g.Ball.Vel.Y *= AirResistance

		if g.Ball.Pos.Z <= 0 {
			g.Ball.Pos.Z = 0
			g.Ball.Vel.Z = -g.Ball.Vel.Z * BounceFactor
			g.Ball.Bounces++
			if g.OnBounce != nil {
				g.OnBounce(g.Ball.Pos.X, g.Ball.Pos.Y)
			}

			// Out of bounds or too many bounces
			if math.Abs(g.Ball.Pos.X) > CourtWidth/2 || math.Abs(g.Ball.Pos.Y) > CourtLength/2 || g.Ball.Bounces > MaxBounces {
				winner := 1
				if g.Ball.Pos.Y > 0 {
					winner = 2
				}
				g.HandlePoint(winner)
				return
			}
		}

		// Net collision
		if math.Abs(g.Ball.Pos.Y) < 10 && g.Ball.Pos.Z < NetHeight {
			winner := 1
			if g.Ball.Vel.Y > 0 {
				winner = 2
			}
			g.HandlePoint(winner)
			return
		}
	}
}

func (g *GameLogic) HandlePoint(winner int) {
	isGamePoint := false
	if winner == 1 {
		if g.PointScore.P1 == 3 {
			g.Score.P1++
			g.PointScore.P1 = 0
			g.PointScore.P2 = 0
			isGamePoint = true
		} else {
			g.PointScore.P1++
		}
	} else {
		if g.PointScore.P2 == 3 {
			g.Score.P2++
			g.PointScore.P1 = 0
			g.PointScore.P2 = 0
			isGamePoint = true
		} else {
			g.PointScore.P2++
		}
	}
	g.State = StateScored
	g.ScoredTimer = 120 // 2 seconds at 60fps

	if g.OnVoice != nil {
		if isGamePoint {
			g.OnVoice(fmt.Sprintf("ゲーム。ゲームカウント %d 対 %d", g.Score.P1, g.Score.P2))
		} else {
			scoreLabels := []string{"0", "15", "30", "40"}
			p1Score := scoreLabels[g.PointScore.P1]
			p2Score := scoreLabels[g.PointScore.P2]

			var sScore, rScore string
			if g.Server == 1 {
				sScore = p1Score
				rScore = p2Score
			} else {
				sScore = p2Score
				rScore = p1Score
			}
			g.OnVoice(fmt.Sprintf("サーバー %s、レシーバー %s", sScore, rScore))
		}
	}
}
