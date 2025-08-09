package main

import (
	"fmt"
	"os"
	"strings"
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

//------------------------------------------------------------------------------
// Model
//------------------------------------------------------------------------------

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

	// Colors
	start colorRGB
	end   colorRGB
}

// Predefined list of popular FIGlet font names supported by go-figure.
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
		start:     colorRGB{138, 43, 226}, // #8A2BE2
		end:       colorRGB{0, 255, 255},  // #00FFFF
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

func (m model) Init() tea.Cmd { return nil }

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
		}
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

	// Colors update when valid
	if c, ok := parseHexColor(m.inputs[1].Value()); ok {
		m.start = c
	}
	if c, ok := parseHexColor(m.inputs[2].Value()); ok {
		m.end = c
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.w == 0 || m.h == 0 {
		return "\n  loading…"
	}

	// Controls panel
	labelStyle := lipgloss.NewStyle().Faint(true)
	box := lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

	ctrlLines := []string{
		labelStyle.Render("Text:") + " " + m.inputs[0].View(),
		labelStyle.Render("Start:") + " " + m.inputs[1].View(),
		labelStyle.Render("End:") + " " + m.inputs[2].View(),
		labelStyle.Render("Font:") + " " + currentFontChip(m.fonts[m.fontIndex]) + "  (←/→ or [/])",
	}
	controls := box.Render(strings.Join(ctrlLines, "\n"))

	// Build colored block-art from ASCII using per-column gradient
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
			c := lerp(m.start, m.end, t)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
			b.WriteString(style.Render("█"))
		}
		rows[y] = b.String()
	}
	art := strings.Join(rows, "\n")

	// Layout: controls on top, art centered below
	gap := strings.Repeat("\n", 1)
	content := controls + gap + art
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, content)
}

func currentFontChip(name string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Background(lipgloss.Color("57")).Padding(0, 1).Render(name)
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
