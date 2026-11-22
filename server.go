package main

import (
	"context"
	_ "embed"
	"html/template"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joelnorton/avantis_utils/internal/clickrate"
)

const (
	serverPort = 9352
	micCount   = 8

	tapRateDuration  = 5 * time.Second
	tapRateLocation  = "fx.rack.0.tl"
	tapRateThreshold = 0.1
)

//go:embed templates/index.html
var indexHTML string

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

// pageData is what the display page renders. Mics is embedded so the mic grid
// keeps the same .Mic field it had before.
type pageData struct {
	Mics
	Devices     []clickrate.Device
	TapRate     *TapRateResult
	TapError    string
	DeviceError string
}

// micServer holds the state rendered by the display.
type micServer struct {
	mu        sync.Mutex
	mics      Mics
	client    *http.Client
	proCfg    APIConfig
	mixingCfg APIConfig

	tapRate   *TapRateResult
	tapError  string
	measuring bool
}

func newMicServer(client *http.Client, mics Mics, proCfg, mixingCfg APIConfig) *micServer {
	return &micServer{mics: mics, client: client, proCfg: proCfg, mixingCfg: mixingCfg}
}

func (s *micServer) index(w http.ResponseWriter, r *http.Request) {
	devices, deviceErr := clickrate.Devices()

	s.mu.Lock()
	defer s.mu.Unlock()

	data := pageData{
		Mics:     s.mics,
		Devices:  devices,
		TapRate:  s.tapRate,
		TapError: s.tapError,
	}
	if deviceErr != nil {
		data.DeviceError = "listing capture devices: " + deviceErr.Error()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, data); err != nil {
		log.Printf("render index: %v", err)
	}
}

func (s *micServer) toggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idx, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/toggle/"))
	if err != nil || idx < 0 || idx >= len(s.mics.Mic) {
		http.NotFound(w, r)
		return
	}

	if s.mics.Mic[idx].Active {
		sendProMacro(s.client, s.proCfg, s.mics.Mic[idx].MacroName)
	} else {
		sendProMacro(s.client, s.proCfg, "Clear_Mics")
	}

	s.mu.Lock()
	s.mics.Mic[idx].Active = !s.mics.Mic[idx].Active
	s.mu.Unlock()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *micServer) setTapRate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.beginMeasure() {
		http.Error(w, "a tap-rate measurement is already running", http.StatusConflict)
		return
	}

	device := strings.TrimSpace(r.FormValue("device"))

	ctx, cancel := context.WithTimeout(r.Context(), tapRateDuration)
	defer cancel()

	result, err := clickrate.Measure(ctx, clickrate.Config{
		Device:    device,
		Duration:  tapRateDuration,
		Threshold: tapRateThreshold,
	})
	if err != nil {
		s.finishMeasure(nil, err.Error())
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	ms := int(math.Round(result.MedianMS))
	if ms <= 0 {
		s.finishMeasure(nil, "measured tap rate is zero; check the input level")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := sendMixingRate(s.client, s.mixingCfg, s.tapRateLocation(), ms); err != nil {
		s.finishMeasure(nil, err.Error())
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.finishMeasure(&TapRateResult{
		MS:         ms,
		BPM:        60000 / float64(ms),
		Peaks:      result.Peaks,
		DeviceName: result.DeviceName,
	}, "")

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// tapRateLocation prefers the location configured in .env.
func (s *micServer) tapRateLocation() string {
	if s.mixingCfg.Location != "" {
		return s.mixingCfg.Location
	}
	return tapRateLocation
}

// beginMeasure claims the single measurement slot, rejecting a second capture
// while one is already running.
func (s *micServer) beginMeasure() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.measuring {
		return false
	}
	s.measuring = true
	return true
}

// finishMeasure releases the measurement slot and records the outcome. A nil
// result keeps the previous tap rate on screen and shows errMsg instead.
func (s *micServer) finishMeasure(result *TapRateResult, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.measuring = false
	if result != nil {
		s.tapRate = result
		s.tapError = ""
		return
	}
	s.tapError = errMsg
}

func startServer(client *http.Client, mics Mics, proCfg, mixingCfg APIConfig) {
	srv := newMicServer(client, mics, proCfg, mixingCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.index)
	mux.HandleFunc("/toggle/", srv.toggle)
	mux.HandleFunc("/setTapRate", srv.setTapRate)

	addr := ":" + strconv.Itoa(serverPort)
	log.Printf("listening on http://localhost:%d", serverPort)
	log.Fatal(http.ListenAndServe(addr, mux))
}
