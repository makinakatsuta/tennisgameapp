//go:build !windows

package audio

import "fmt"

func (e *Engine) PlayVoice(text string) {
	// Fallback: Just print to console or use a cross-platform TTS library if available
	fmt.Println("Voice Call:", text)
}
