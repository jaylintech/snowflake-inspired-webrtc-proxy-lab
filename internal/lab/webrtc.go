package lab

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/pion/webrtc/v3"
)

const DefaultSTUN = "stun:stun.l.google.com:19302"

type PeerConnectionOptions struct {
	TransportPreset string
	STUNServers    string
	TURNServers    string
	TURNUsername   string
	TURNCredential string
	ICEPolicy      string
	ICEPortMin     uint
	ICEPortMax     uint
	AdvertiseIP    string
}

// ApplyTransportPreset configures STUN, TURN, and ICE policy according to a named transport preset.
// Supported presets:
// - "direct": no STUN, no TURN, ICEPolicy "all" (local/direct host only)
// - "stun": DefaultSTUN, no TURN, ICEPolicy "all"
// - "turn-udp": turn:<host>:3478, ICEPolicy "relay"
// - "turn-tcp": turn:<host>:3478?transport=tcp, ICEPolicy "relay"
// - "turns-tls": turns:<host>:443?transport=tcp, ICEPolicy "relay"
func ApplyTransportPreset(options *PeerConnectionOptions, preset string, defaultTurnHost string) error {
	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset == "" {
		return nil
	}
	options.TransportPreset = preset
	turnHost := defaultTurnHost
	if turnHost == "" {
		turnHost = "turn.lab.example"
	}

	switch preset {
	case "direct":
		options.STUNServers = ""
		options.TURNServers = ""
		options.ICEPolicy = "all"
	case "stun":
		options.STUNServers = DefaultSTUN
		options.TURNServers = ""
		options.ICEPolicy = "all"
	case "turn-udp":
		options.STUNServers = ""
		options.TURNServers = fmt.Sprintf("turn:%s:3478", turnHost)
		options.ICEPolicy = "relay"
	case "turn-tcp":
		options.STUNServers = ""
		options.TURNServers = fmt.Sprintf("turn:%s:3478?transport=tcp", turnHost)
		options.ICEPolicy = "relay"
	case "turns-tls":
		options.STUNServers = ""
		options.TURNServers = fmt.Sprintf("turns:%s:443?transport=tcp", turnHost)
		options.ICEPolicy = "relay"
	default:
		return fmt.Errorf("unknown transport preset %q; must be direct, stun, turn-udp, turn-tcp, or turns-tls", preset)
	}
	return nil
}

// NewWebRTCConfig returns a minimal WebRTC configuration for a lab run.
// Pass an empty stunCSV value to disable external STUN and test only local ICE.
func NewWebRTCConfig(stunCSV string) webrtc.Configuration {
	config, _ := NewWebRTCConfigWithOptions(PeerConnectionOptions{STUNServers: stunCSV})
	return config
}

// NewWebRTCConfigWithOptions returns a validated STUN/TURN configuration.
func NewWebRTCConfigWithOptions(options PeerConnectionOptions) (webrtc.Configuration, error) {
	stunURLs := splitCSV(options.STUNServers)
	turnURLs := splitCSV(options.TURNServers)
	if err := validateICEServerURLs(stunURLs, "stun", "stuns"); err != nil {
		return webrtc.Configuration{}, err
	}
	if err := validateICEServerURLs(turnURLs, "turn", "turns"); err != nil {
		return webrtc.Configuration{}, err
	}

	username := strings.TrimSpace(options.TURNUsername)
	credential := strings.TrimSpace(options.TURNCredential)
	if (username == "") != (credential == "") {
		return webrtc.Configuration{}, fmt.Errorf("TURN username and credential must both be set, or both be empty")
	}

	config := webrtc.Configuration{}
	if len(stunURLs) > 0 {
		config.ICEServers = append(config.ICEServers, webrtc.ICEServer{URLs: stunURLs})
	}
	if len(turnURLs) > 0 {
		config.ICEServers = append(config.ICEServers, webrtc.ICEServer{
			URLs:       turnURLs,
			Username:   username,
			Credential: credential,
		})
	}

	switch strings.ToLower(strings.TrimSpace(options.ICEPolicy)) {
	case "", "all":
		config.ICETransportPolicy = webrtc.ICETransportPolicyAll
	case "relay":
		config.ICETransportPolicy = webrtc.ICETransportPolicyRelay
	default:
		return webrtc.Configuration{}, fmt.Errorf("ICE policy must be all or relay")
	}

	return config, nil
}

// NewPeerConnection creates a PeerConnection with optional local ICE UDP port bounds
// and an optional public IP to advertise for host candidates behind a port forward.
// Pass 0 for both port values to use Pion's default dynamic UDP port behavior.
func NewPeerConnection(stunCSV string, icePortMin, icePortMax uint, advertiseIP string) (*webrtc.PeerConnection, error) {
	return NewPeerConnectionWithOptions(PeerConnectionOptions{
		STUNServers: stunCSV,
		ICEPortMin:  icePortMin,
		ICEPortMax:  icePortMax,
		AdvertiseIP: advertiseIP,
	})
}

// PeerConnectionOptionsFromEnvironment adds the Part 2 TURN settings while
// preserving the explicit STUN, port-range, and advertised-IP command flags.
func PeerConnectionOptionsFromEnvironment(stunCSV string, icePortMin, icePortMax uint, advertiseIP string) PeerConnectionOptions {
	opts := PeerConnectionOptions{
		STUNServers:    stunCSV,
		TURNServers:    os.Getenv("LAB_TURN_URLS"),
		TURNUsername:   os.Getenv("LAB_TURN_USERNAME"),
		TURNCredential: os.Getenv("LAB_TURN_CREDENTIAL"),
		ICEPolicy:      os.Getenv("LAB_ICE_POLICY"),
		ICEPortMin:     icePortMin,
		ICEPortMax:     icePortMax,
		AdvertiseIP:    advertiseIP,
	}
	if envTransport := os.Getenv("LAB_TRANSPORT"); envTransport != "" {
		_ = ApplyTransportPreset(&opts, envTransport, os.Getenv("LAB_TURN_HOST"))
	}
	return opts
}

// NewPeerConnectionWithOptions creates a PeerConnection with validated STUN,
// TURN, relay-policy, port-range, and advertised-IP settings.
func NewPeerConnectionWithOptions(options PeerConnectionOptions) (*webrtc.PeerConnection, error) {
	config, err := NewWebRTCConfigWithOptions(options)
	if err != nil {
		return nil, err
	}
	advertiseIP := strings.TrimSpace(options.AdvertiseIP)
	if options.ICEPortMin == 0 && options.ICEPortMax == 0 && advertiseIP == "" {
		return webrtc.NewPeerConnection(config)
	}
	if options.ICEPortMin == 0 || options.ICEPortMax == 0 {
		return nil, fmt.Errorf("ice port min and max must both be set, or both be 0")
	}
	if options.ICEPortMin > 65535 || options.ICEPortMax > 65535 {
		return nil, fmt.Errorf("ice port values must be between 1 and 65535")
	}
	if advertiseIP != "" && net.ParseIP(advertiseIP) == nil {
		return nil, fmt.Errorf("advertise IP must be a valid IPv4 or IPv6 address")
	}

	var settingEngine webrtc.SettingEngine
	if err := settingEngine.SetEphemeralUDPPortRange(uint16(options.ICEPortMin), uint16(options.ICEPortMax)); err != nil {
		return nil, fmt.Errorf("set ICE UDP port range: %w", err)
	}
	if advertiseIP != "" {
		settingEngine.SetNAT1To1IPs([]string{advertiseIP}, webrtc.ICECandidateTypeHost)
	}

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	return api.NewPeerConnection(config)
}

func validateICEServerURLs(urls []string, allowedSchemes ...string) error {
	allowed := make(map[string]struct{}, len(allowedSchemes))
	for _, scheme := range allowedSchemes {
		allowed[scheme] = struct{}{}
	}

	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" {
			return fmt.Errorf("invalid ICE server URL %q", raw)
		}
		if _, ok := allowed[strings.ToLower(parsed.Scheme)]; !ok {
			return fmt.Errorf("ICE server URL %q must use %s", raw, strings.Join(allowedSchemes, " or "))
		}
	}
	return nil
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
