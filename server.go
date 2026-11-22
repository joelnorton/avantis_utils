package main

import (
	_ "embed"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const (
	serverPort = 9352
	micCount   = 8
)

//go:embed templates/index.html
var indexHTML string

var indexTemplate = template.Must(template.New("index").Parse(indexHTML))

// micServer holds the state rendered by the display.
type micServer struct {
	mu        sync.Mutex
	mics      Mics
	client    *http.Client
	proCfg    APIConfig
	mixingCfg APIConfig
}

func newMicServer(client *http.Client, mics Mics, proCfg, mixingCfg APIConfig) *micServer {
	return &micServer{mics: mics, client: client, proCfg: proCfg, mixingCfg: mixingCfg}
}

func (s *micServer) index(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTemplate.Execute(w, s.mics); err != nil {
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

	rate, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/setTapRate/"))
	if err != nil || rate < 0 {
		http.Error(w, "tap rate must be a non-negative number", http.StatusBadRequest)
		return
	}

	sendMixingRate(s.client, s.mixingCfg, rate)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func startServer(client *http.Client, mics Mics, proCfg, mixingCfg APIConfig) {
	srv := newMicServer(client, mics, proCfg, mixingCfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.index)
	mux.HandleFunc("/toggle/", srv.toggle)
	mux.HandleFunc("/setTapRate/", srv.setTapRate)

	addr := ":" + strconv.Itoa(serverPort)
	log.Printf("listening on http://localhost:%d", serverPort)
	log.Fatal(http.ListenAndServe(addr, mux))
}
