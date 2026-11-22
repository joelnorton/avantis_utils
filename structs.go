package main

type APIConfig struct {
	Server string
	Port   int
	Location string
}

type MicStatus struct {
	Name      string
	Active    bool
	MacroName string
}

type Mics struct {
	Mic []MicStatus
}
