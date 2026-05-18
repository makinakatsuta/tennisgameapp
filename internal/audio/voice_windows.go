//go:build windows

package audio

import (
	"log"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func (e *Engine) PlayVoice(text string) {
	go func() {
		ole.CoInitialize(0)
		defer ole.CoUninitialize()

		unknown, err := oleutil.CreateObject("SAPI.SpVoice")
		if err != nil {
			log.Println("SAPI.SpVoice Error:", err)
			return
		}
		voice, _ := unknown.QueryInterface(ole.IID_IDispatch)
		defer voice.Release()

		oleutil.CallMethod(voice, "Speak", text)
	}()
}
