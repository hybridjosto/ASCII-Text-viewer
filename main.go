package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	figure "github.com/common-nighthawk/go-figure"
)

// Build & run:
//   go mod init glamdm
//   go get github.com/charmbracelet/bubbletea \
//          github.com/charmbracelet/bubbles \
//          github.com/charmbracelet/lipgloss \
//          github.com/common-nighthawk/go-figure
//   go run .
// Quit with q or Ctrl+C.

// Notes:
// - Cycle fonts with ←/→ (left/right) or [/] .
// - Edit fields with Tab to move focus.
// - Text updates live; colors apply as you type valid hex (e.g. #8A2BE2).
// - Press 'm' to toggle render mode (BLOCK/GLYPH/LIGHT/DOTS).
// - Press 'a' to toggle animated hue cycling. Use '+' and '-' to change speed.

//------------------------------------------------------------------------------
// Model & Types
//------------------------------------------------------------------------------

type renderMode int

const (
	modeBlock renderMode = iota // Replace glyphs with full block (█)
	modeGlyph                   // Keep original FIGlet glyphs
	modeLight                   // Medium block (▓)
	modeDots                    // Dotted look (·)
)

var modeNames = []string{"BLOCK █", "GLYPH", "LIGHT ▓", "DOTS ·"}

type colorRGB struct{ R, G, B int }

func (c colorRGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func lerp(a, b colorRGB, t float64) colorRGB {
	return colorRGB{
		R: int(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: int(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: int(float64(a.B) + (float64(b.B)-float64(a.B))*t),
	}
}

func parseHexColor(s string) (colorRGB, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return colorRGB{}, false
	}
	s = s[1:]
	var r, g, b int
	switch len(s) {
	case 6:
		_, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
		return colorRGB{r, g, b}, err == nil
	case 3:
		var r1, g1, b1 byte
		_, err := fmt.Sscanf(s, "%1x%1x%1x", &r1, &g1, &b1)
		return colorRGB{int(r1) * 17, int(g1) * 17, int(b1) * 17}, err == nil
	default:
		return colorRGB{}, false
	}
}

// HSV helpers for hue rotation
func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func rgbToHsv(c colorRGB) (h, s, v float64) {
	r := float64(c.R) / 255.0
	g := float64(c.G) / 255.0
	b := float64(c.B) / 255.0
	maxv := math.Max(r, math.Max(g, b))
	minv := math.Min(r, math.Min(g, b))
	d := maxv - minv
	v = maxv
	if maxv == 0 { // black
		return 0, 0, 0
	}
	s = 0
	if maxv != 0 {
		s = d / maxv
	}
	if d == 0 {
		h = 0
	} else {
		switch maxv {
		case r:
			h = (g - b) / d
			if g < b {
				h += 6
			}
		case g:
			h = (b-r)/d + 2
		case b:
			h = (r-g)/d + 4
		}
		h *= 60
	}
	return
}

func hsvToRgb(h, s, v float64) colorRGB {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60.0, 2)-1))
	m := v - c
	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1, b1 = c, x, 0
	case h < 120:
		r1, g1, b1 = x, c, 0
	case h < 180:
		r1, g1, b1 = 0, c, x
	case h < 240:
		r1, g1, b1 = 0, x, c
	case h < 300:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}
	return colorRGB{int((r1 + m) * 255), int((g1 + m) * 255), int((b1 + m) * 255)}
}

func rotateHue(c colorRGB, delta float64) colorRGB {
	h, s, v := rgbToHsv(c)
	return hsvToRgb(h+delta, s, v)
}

// Messages for animation tick
type tickMsg time.Time

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	w, h int

	// Controls
	inputs     []textinput.Model // 0=text, 1=start hex, 2=end hex
	focusIndex int

	fonts     []string
	fontIndex int

	// Render cache
	artLines []string
	maxWidth int

	// Colors (base are user-chosen; effective may be hue-rotated)
	baseStart colorRGB
	baseEnd   colorRGB

	// Mode
	mode renderMode

	// Animation
	animate  bool
	hueShift float64       // degrees
	stepDeg  float64       // degrees per tick
	interval time.Duration // tick interval
}

// FIGlet fonts list
var figFonts = []string{
	"standard", "big", "doom", "slant", "shadow", "block", "banner", "larry3d", "speed", "smslant", "small", "isometric1",
	"3-d", "3x5", "5lineoblique", "acrobatic", "alligator", "alligator2", "alphabet",
	"avatar", "banner3-D", "banner3", "banner4", "barbwire", "basic", "bell", "bigchief",
	"binary", "bubble", "bulbhead", "calgphy2", "caligraphy", "catwalk", "chunky",
	"coinstak", "colossal", "computer", "contessa", "contrast", "cosmic", "cosmike",
	"cricket", "cursive", "cyberlarge", "cybermedium", "cybersmall", "diamond", "digital", "doh", "dotmatrix", "drpepper",
	"eftichess", "eftifont", "eftipiti", "eftirobot", "eftitalic", "eftiwall", "eftiwater",
	"epic", "fender", "fourtops", "fuzzy", "goofy", "gothic", "graffiti", "hollywood",
	"invita", "isometric2", "isometric3", "isometric4", "italic", "ivrit", "jazmine",
	"jerusalem", "katakana", "kban", "lcd", "lean", "letters", "linux", "lockergnome",
	"madrid", "marquee", "maxfour", "mike", "mini", "mirror", "mnemonic", "morse",
	"moscow", "nancyj-fancy", "nancyj-underlined", "nancyj", "nipples", "ntgreek", "o8",
	"ogre", "pawp", "peaks", "pebbles", "pepper", "poison", "puffy", "pyramid", "rectangles",
	"relief", "relief2", "rev", "roman", "rot13", "rounded", "rowancap", "rozzo", "runic",
	"runyc", "sblood", "script", "serifcap", "short", "slide", "slscript", "smisome1", "smkeyboard",
	"smscript", "smshadow", "smtengwar", "stampatello", "starwars", "stellar", "stop",
	"straight", "tanja", "tengwar", "term", "thick", "thin", "threepoint", "ticks", "ticksslant",
	"tinker-toy", "tombstone", "trek", "tsalagi", "twopoint", "univers", "usaflag", "wavy",
	"weird",
}

func newTextInput(placeholder string, value string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.Prompt = ""
	ti.CharLimit = 256
	ti.Width = max(12, utf8.RuneCountInString(value)+4)
	return ti
}

func newModel() model {
	m := model{
		fonts:     figFonts,
		fontIndex: 0,
		baseStart: colorRGB{138, 43, 226}, // #8A2BE2
		baseEnd:   colorRGB{0, 255, 255},  // #00FFFF
		mode:      modeGlyph,              // default: keep original glyphs
		animate:   true,
		hueShift:  0,
		stepDeg:   3,                     // degrees per tick
		interval:  60 * time.Millisecond, // ~16 FPS
	}
	m.inputs = []textinput.Model{
		newTextInput("text", "glam dm"),
		newTextInput("start hex", "#8A2BE2"),
		newTextInput("end hex", "#00FFFF"),
	}
	m.inputs[0].Focus()
	m.rebuildArt()
	return m
}

func (m *model) rebuildArt() {
	txt := m.inputs[0].Value()
	font := m.fonts[m.fontIndex]
	fig := figure.NewFigure(txt, font, true)
	lines := strings.Split(strings.TrimRight(fig.String(), "\n"), "\n")
	maxW := 0
	for _, l := range lines {
		if len(l) > maxW {
			maxW = len(l)
		}
	}
	m.artLines = lines
	m.maxWidth = maxW
}

//------------------------------------------------------------------------------
// Bubble Tea
//------------------------------------------------------------------------------

func (m model) Init() tea.Cmd {
	if m.animate {
		return tickEvery(m.interval)
	}
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			if msg.String() == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}
			if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs) - 1
			}
			if m.focusIndex >= len(m.inputs) {
				m.focusIndex = 0
			}
			for i := range m.inputs {
				if i == m.focusIndex {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, nil
		case "left", "[":
			m.fontIndex = (m.fontIndex - 1 + len(m.fonts)) % len(m.fonts)
			m.rebuildArt()
			return m, nil
		case "right", "]":
			m.fontIndex = (m.fontIndex + 1) % len(m.fonts)
			m.rebuildArt()
			return m, nil
		case "m":
			m.mode = (m.mode + 1) % renderMode(len(modeNames))
			return m, nil
		case "a":
			m.animate = !m.animate
			if m.animate {
				return m, tickEvery(m.interval)
			}
			return m, nil
		case "+", "=":
			m.stepDeg = math.Min(30, m.stepDeg+0.5)
			return m, nil
		case "-", "_":
			m.stepDeg = math.Max(0.5, m.stepDeg-0.5)
			return m, nil
		}
	case tickMsg:
		if m.animate {
			m.hueShift = math.Mod(m.hueShift+m.stepDeg, 360)
			return m, tickEvery(m.interval)
		}
		return m, nil
	}

	// Update inputs and live-apply changes
	var cmds []tea.Cmd
	for i := range m.inputs {
		var cmd tea.Cmd
		m.inputs[i], cmd = m.inputs[i].Update(msg)
		cmds = append(cmds, cmd)
	}

	// Text changes rebuild art
	m.rebuildArt()

	// Colors update when valid (these are bases for hue rotation)
	if c, ok := parseHexColor(m.inputs[1].Value()); ok {
		m.baseStart = c
	}
	if c, ok := parseHexColor(m.inputs[2].Value()); ok {
		m.baseEnd = c
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.w == 0 || m.h == 0 {
		return "\n  loading…"
	}

	// Effective colors (possibly hue-rotated)
	effStart := m.baseStart
	effEnd := m.baseEnd
	if m.animate {
		effStart = rotateHue(effStart, m.hueShift)
		effEnd = rotateHue(effEnd, m.hueShift)
	}

	// Controls panel
	labelStyle := lipgloss.NewStyle().Faint(true)
	box := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

	animState := "off"
	if m.animate {
		animState = fmt.Sprintf("on (%.1f°/tick)", m.stepDeg)
	}
	ctrlLines := []string{
		labelStyle.Render("Text:") + " " + m.inputs[0].View(),
		labelStyle.Render("Start:") + " " + m.inputs[1].View(),
		labelStyle.Render("End:") + " " + m.inputs[2].View(),
		labelStyle.Render("Font:") + " " + currentChip(m.fonts[m.fontIndex], "212", "57") + "  (←/→ or [/])",
		labelStyle.Render("Mode:") + " " + currentChip(modeNames[m.mode], "118", "237") + "  (m)",
		labelStyle.Render("Hue cycle:") + " " + currentChip(animState, "51", "240") + "  (a, +/-)",
	}
	controls := box.Render(strings.Join(ctrlLines, "\n"))

	// Build colored art from ASCII using per-column gradient & render modes
	rows := make([]string, len(m.artLines))
	for y, line := range m.artLines {
		if len(line) < m.maxWidth {
			line += strings.Repeat(" ", m.maxWidth-len(line))
		}
		var b strings.Builder
		for x := 0; x < m.maxWidth; x++ {
			ch := line[x]
			if ch == ' ' {
				b.WriteByte(' ')
				continue
			}
			t := 0.0
			if m.maxWidth > 1 {
				t = float64(x) / float64(m.maxWidth-1)
			}
			c := lerp(effStart, effEnd, t)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
			switch m.mode {
			case modeBlock:
				b.WriteString(style.Render("█"))
			case modeLight:
				b.WriteString(style.Render("▓"))
			case modeDots:
				b.WriteString(style.Render("·"))
			case modeGlyph:
				b.WriteString(style.Render(string(ch)))
			}
		}
		rows[y] = b.String()
	}
	art := strings.Join(rows, "\n")

	// Layout: controls on top, art centered below
	gap := strings.Repeat("\n", 1)
	content := controls + gap + art
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, content)
}

func currentChip(name, fg, bg string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg)).Padding(0, 1).Render(name)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
