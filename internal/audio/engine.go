package audio

import (
	"bytes"
	"io"
	"log"
	"math"
	"math/rand"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/tphakala/go-m4a/aacm4a"
)

const (
	SampleRate     = 44100
	ListenerHeight = 60.0
)

type Engine struct {
	audioContext *audio.Context
	rattlePlayer *audio.Player
	rattleStream *RattleStream
	racketPCM    []byte
}

func NewEngine(racketM4A []byte) *Engine {
	ctx := audio.NewContext(SampleRate)
	stream := &RattleStream{
		sampleRate: SampleRate,
		lfoFreq:    8.0,    // Slower LFO for a smoother sound
		noiseFreq:  1200.0, // Lower frequency for a softer "whoosh"
	}
	player, _ := ctx.NewPlayer(stream)
	player.Play()

	e := &Engine{
		audioContext: ctx,
		rattlePlayer: player,
		rattleStream: stream,
	}
	e.racketPCM = decodeRacketSound(racketM4A)
	return e
}

func decodeRacketSound(data []byte) []byte {
	decoder, info, err := aacm4a.NewDecoder(bytes.NewReader(data))
	if err != nil {
		log.Printf("audio: could not load racket sound: %v", err)
		return nil
	}
	pcm, err := io.ReadAll(decoder)
	if err != nil {
		log.Printf("audio: could not decode racket sound: %v", err)
		return nil
	}

	// Ebiten expects interleaved 16-bit PCM. Remove AAC encoder priming when
	// the container provides it, while retaining the original sample rate.
	bytesPerFrame := info.Channels * 2
	trim := int(info.EncoderDelay) * bytesPerFrame
	if trim >= len(pcm) {
		return nil
	}
	if trim > 0 {
		pcm = pcm[trim:]
	}
	return pcm
}

func (e *Engine) UpdateBallSound(x, y, z, vx, vy, vz, listenerX, listenerY float64) {
	speed := math.Sqrt(vx*vx + vy*vy + vz*vz)
	dx := x - listenerX
	dy := y - listenerY
	dz := z - ListenerHeight
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	pan := math.Max(-1, math.Min(1, dx/305.0))

	targetGain := 0.0
	if speed > 1.0 {
		// A moving listener makes the ball louder when it is nearby and quieter
		// when it is across the court.
		attenuation := 1.0 / (1.0 + distance/500.0)
		targetGain = 0.12 * attenuation * math.Min(speed/15.0, 1.0)
	}

	e.rattleStream.SetGain(targetGain)
	e.rattleStream.SetPan(pan)
}

func (e *Engine) PlayBounce(x, y, listenerX, listenerY float64) {
	// A low thud for the bounce
	dx := x - listenerX
	dy := y - listenerY
	distance := math.Sqrt(dx*dx + dy*dy)
	attenuation := 1.0 / (1.0 + distance/500.0)
	pan := math.Max(-1, math.Min(1, dx/305.0))
	go e.playBounceSound(pan, 0.55*attenuation)
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

// PlayHit plays a hit at a court position relative to the moving player.
func (e *Engine) PlayHit(isPlayer1 bool, x, y, listenerX, listenerY float64) {
	if isPlayer1 {
		if len(e.racketPCM) > 0 {
			p := e.audioContext.NewPlayerFromBytes(spatialize(e.racketPCM, x, y, listenerX, listenerY))
			p.Play()
			return
		}
		// Fall back to the synthesized impact if the asset cannot be decoded.
	}
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

// spatialize converts court coordinates into a simple 3D audio cue. The
// player is the listener, so horizontal position controls stereo pan and
// front/back position controls distance attenuation.
func spatialize(pcm []byte, sourceX, sourceY, listenerX, listenerY float64) []byte {
	if len(pcm)%4 != 0 {
		return pcm
	}

	dx := sourceX - listenerX
	dy := sourceY - listenerY
	distance := math.Sqrt(dx*dx + dy*dy)
	pan := math.Max(-1, math.Min(1, dx/305.0))
	attenuation := 1.0 / (1.0 + distance/500.0)
	leftGain := attenuation * math.Sqrt((1-pan)*0.5)
	rightGain := attenuation * math.Sqrt((1+pan)*0.5)

	out := make([]byte, len(pcm))
	for i := 0; i < len(pcm); i += 4 {
		left := int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8)
		right := int16(uint16(pcm[i+2]) | uint16(pcm[i+3])<<8)
		outLeft := int16(math.Max(-32768, math.Min(32767, float64(left)*leftGain)))
		outRight := int16(math.Max(-32768, math.Min(32767, float64(right)*rightGain)))
		out[i] = byte(outLeft)
		out[i+1] = byte(uint16(outLeft) >> 8)
		out[i+2] = byte(outRight)
		out[i+3] = byte(uint16(outRight) >> 8)
	}
	return out
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
		tone := 2.0 / math.Pi * math.Asin(math.Sin(phase))

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
			val = 2.0 / math.Pi * math.Asin(math.Sin(phase))
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
