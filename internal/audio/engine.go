package audio

import (
	"math"
	"math/rand"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

const (
	SampleRate = 44100
)

type Engine struct {
	audioContext *audio.Context
	rattlePlayer *audio.Player
	rattleStream *RattleStream
}

func NewEngine() *Engine {
	ctx := audio.NewContext(SampleRate)
	stream := &RattleStream{
		sampleRate: SampleRate,
		lfoFreq:    8.0,   // Slower LFO for a smoother sound
		noiseFreq:  1200.0, // Lower frequency for a softer "whoosh"
	}
	player, _ := ctx.NewPlayer(stream)
	player.Play()

	return &Engine{
		audioContext: ctx,
		rattlePlayer: player,
		rattleStream: stream,
	}
}

func (e *Engine) UpdateBallSound(x, y, z, vx, vy, vz float64) {
	speed := math.Sqrt(vx*vx + vy*vy + vz*vz)
	distance := 1.0 - math.Abs(y/670.0)
	if distance < 0 {
		distance = 0
	}

	targetGain := 0.0
	if speed > 1.0 {
		// Reduced gain further (0.1 -> 0.08) as per user request (20% reduction)
		targetGain = 0.08 * distance * math.Min(speed/15.0, 1.0)
	}

	e.rattleStream.SetGain(targetGain)
	e.rattleStream.SetPan(x / 305.0)
}

func (e *Engine) PlayBounce(x, y float64) {
	// A low thud for the bounce
	gain := 0.4
	if y > 0 { // Ball is on player's side
		gain *= 1.3
	}
	go e.playBounceSound(x/305.0, gain)
}

func (e *Engine) playBounceSound(pan, gain float64) {
	duration := 0.1
	numSamples := int(SampleRate * duration)
	buf := make([]byte, numSamples*4)
	for i := 0; i < numSamples; i++ {
		t := float64(i) / SampleRate
		progress := t / duration
		
		// Low frequency thump (Sine)
		freq := 120.0 * (1.0 - progress*0.5)
		val := math.Sin(2 * math.Pi * freq * t)
		
		// Envelope with sharp decay
		amp := math.Exp(-progress * 15.0)
		
		sample := int16(val * amp * gain * 32767)

		left := float64(sample) * math.Min(1.0-pan, 1.0)
		right := float64(sample) * math.Min(1.0+pan, 1.0)

		lVal, rVal := int16(left), int16(right)
		buf[i*4] = byte(lVal)
		buf[i*4+1] = byte(lVal >> 8)
		buf[i*4+2] = byte(rVal)
		buf[i*4+3] = byte(rVal >> 8)
	}
	p := e.audioContext.NewPlayerFromBytes(buf)
	p.Play()
}

func (e *Engine) PlayServe(isPlayer1 bool) {
	freq := 600.0
	if isPlayer1 {
		freq = 550.0
	}
	// A bit more punchy for serve
	go e.playImpact(freq, 0.15, 0.8)
}

func (e *Engine) PlayHit(isPlayer1 bool) {
	freq := 500.0
	noiseAmp := 0.5
	if isPlayer1 {
		freq = 450.0
	} else {
		// Further increase opponent's hit sound by another 80% (1.6 * 1.8 = 2.88)
		noiseAmp *= 2.88
	}
	go e.playImpact(freq, 0.1, noiseAmp)
}

func (e *Engine) PlaySwing() {
	// A very subtle "whoosh" sound for the racket swing
	go e.playNoiseSweep(800.0, 1500.0, 0.08, 0.05)
}

func (e *Engine) playImpact(freq, duration, noiseAmp float64) {
	numSamples := int(SampleRate * duration)
	buf := make([]byte, numSamples*4)
	for i := 0; i < numSamples; i++ {
		t := float64(i) / SampleRate
		progress := t / duration
		
		// Tone component (Triangle)
		phase := 2 * math.Pi * freq * t
		tone := 2.0/math.Pi * math.Asin(math.Sin(phase))
		
		// Noise component
		noise := (rand.Float64()*2.0 - 1.0)
		
		// Envelope
		amp := math.Exp(-progress * 10.0) // Sharp decay
		
		val := (tone*0.6 + noise*noiseAmp) * amp * 0.5
		sample := int16(val * 32767)

		buf[i*4] = byte(sample)
		buf[i*4+1] = byte(sample >> 8)
		buf[i*4+2] = byte(sample)
		buf[i*4+3] = byte(sample >> 8)
	}
	p := e.audioContext.NewPlayerFromBytes(buf)
	p.Play()
}

func (e *Engine) playTone(startFreq, endFreq, duration float64, pan float64, waveType string) {
	numSamples := int(SampleRate * duration)
	buf := make([]byte, numSamples*4)
	for i := 0; i < numSamples; i++ {
		t := float64(i) / SampleRate
		progress := t / duration
		freq := startFreq + (endFreq-startFreq)*progress
		
		var val float64
		phase := 2 * math.Pi * freq * t
		if waveType == "sine" {
			val = math.Sin(phase)
		} else { // triangle
			val = 2.0/math.Pi * math.Asin(math.Sin(phase))
		}

		// Envelope
		amp := 1.0 - progress
		sample := int16(val * amp * 0.5 * 32767)

		left := float64(sample) * math.Min(1.0-pan, 1.0)
		right := float64(sample) * math.Min(1.0+pan, 1.0)

		lVal, rVal := int16(left), int16(right)
		buf[i*4] = byte(lVal)
		buf[i*4+1] = byte(lVal >> 8)
		buf[i*4+2] = byte(rVal)
		buf[i*4+3] = byte(rVal >> 8)
	}
	p := e.audioContext.NewPlayerFromBytes(buf)
	p.Play()
}

func (e *Engine) playNoiseSweep(startFreq, endFreq, duration, volume float64) {
	numSamples := int(SampleRate * duration)
	buf := make([]byte, numSamples*4)
	for i := 0; i < numSamples; i++ {
		t := float64(i) / SampleRate
		progress := t / duration
		// Simplified noise sweep: just random with envelope
		val := (rand.Float64()*2.0 - 1.0) * (1.0 - progress) * volume
		sample := int16(val * 32767)
		buf[i*4] = byte(sample)
		buf[i*4+1] = byte(sample >> 8)
		buf[i*4+2] = byte(sample)
		buf[i*4+3] = byte(sample >> 8)
	}
	p := e.audioContext.NewPlayerFromBytes(buf)
	p.Play()
}

type RattleStream struct {
	sampleRate int
	lfoFreq    float64
	noiseFreq  float64
	gain       float64
	pan        float64
	lfoPhase   float64
	mu         sync.Mutex
}

func (s *RattleStream) SetGain(g float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gain = g
}

func (s *RattleStream) SetPan(p float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pan = p
}

func (s *RattleStream) Read(buf []byte) (int, error) {
	s.mu.Lock()
	gain := s.gain
	pan := s.pan
	s.mu.Unlock()

	numSamples := len(buf) / 4
	for i := 0; i < numSamples; i++ {
		// Softer LFO modulation
		lfo := 0.7 + 0.3*math.Sin(2*math.Pi*s.lfoFreq*s.lfoPhase)
		s.lfoPhase += 1.0 / float64(s.sampleRate)

		// Smoother, quieter noise for flight sound (Reduced from 0.4 to 0.32)
		noise := (rand.Float64()*2.0 - 1.0) * lfo * gain * 0.32
		val := int16(noise * 32767)

		left := float64(val) * math.Min(1.0-pan, 1.0)
		right := float64(val) * math.Min(1.0+pan, 1.0)

		lVal, rVal := int16(left), int16(right)
		buf[i*4] = byte(lVal)
		buf[i*4+1] = byte(lVal >> 8)
		buf[i*4+2] = byte(rVal)
		buf[i*4+3] = byte(rVal >> 8)
	}
	return len(buf), nil
}

func (s *RattleStream) Close() error { return nil }
