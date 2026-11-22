package main

type APIConfig struct {
	Server   string
	Port     int
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

// TapRateResult is the tap rate that was measured and sent to the mixing
// server, kept so the page can display it after the redirect.
type TapRateResult struct {
	MS         int
	BPM        float64
	Peaks      int
	DeviceName string
}
