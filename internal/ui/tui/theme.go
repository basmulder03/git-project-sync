package tui

import (
	"fmt"
	"strings"
)

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func visibleLen(s string) int {
	n := 0
	inEsc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if !inEsc {
			if ch == 0x1b {
				inEsc = true
				continue
			}
			n++
			continue
		}
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			inEsc = false
		}
	}
	return n
}

func padRightANSI(s string, width int) string {
	if width <= 0 {
		return ""
	}
	vl := visibleLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}

func box(title string, body []string, width int) string {
	return boxWithFocus(title, body, width, false)
}

func boxWithFocus(title string, body []string, width int, focused bool) string {
	if width < 24 {
		width = 24
	}
	inner := width - 2
	b := &strings.Builder{}
	t := " " + clip(title, inner-2) + " "
	leftTop := "┌"
	rightTop := "┐"
	leftSide := "│"
	rightSide := "│"
	leftBottom := "└"
	rightBottom := "┘"
	hRule := "─"
	if focused {
		leftTop = accent(leftTop)
		rightTop = accent(rightTop)
		leftSide = accent(leftSide)
		rightSide = accent(rightSide)
		leftBottom = accent(leftBottom)
		rightBottom = accent(rightBottom)
		hRule = accent(hRule)
	} else {
		leftTop = muted(leftTop)
		rightTop = muted(rightTop)
		leftSide = muted(leftSide)
		rightSide = muted(rightSide)
		leftBottom = muted(leftBottom)
		rightBottom = muted(rightBottom)
		hRule = muted(hRule)
	}
	top := leftTop + t + strings.Repeat(hRule, inner-visibleLen(t)) + rightTop
	fmt.Fprintln(b, top)
	if len(body) == 0 {
		fmt.Fprintf(b, "%s%s%s\n", leftSide, strings.Repeat(" ", inner), rightSide)
	} else {
		for _, line := range body {
			fmt.Fprintf(b, "%s%s%s\n", leftSide, padRightANSI(clip(line, inner), inner), rightSide)
		}
	}
	fmt.Fprintf(b, "%s%s%s", leftBottom, strings.Repeat(hRule, inner), rightBottom)
	return b.String()
}

func joinColumns(left, right string, totalWidth, gap int) string {
	if totalWidth < 80 {
		if right == "" {
			return left
		}
		return left + "\n\n" + right
	}
	if gap < 1 {
		gap = 1
	}
	leftW := (totalWidth - gap) / 2
	rightW := totalWidth - gap - leftW
	if leftW < 20 || rightW < 20 {
		if right == "" {
			return left
		}
		return left + "\n\n" + right
	}

	leftLines := splitLines(left)
	rightLines := splitLines(right)
	rows := len(leftLines)
	if len(rightLines) > rows {
		rows = len(rightLines)
	}
	if rows == 0 {
		return ""
	}
	b := &strings.Builder{}
	spacer := strings.Repeat(" ", gap)
	for i := 0; i < rows; i++ {
		l := ""
		r := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		fmt.Fprintf(b, "%s%s%s\n", padRightANSI(l, leftW), spacer, padRightANSI(r, rightW))
	}
	return strings.TrimRight(b.String(), "\n")
}

func colorFG(code int, s string) string {
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", code, s)
}

func colorBG(fg, bg int, s string) string {
	return fmt.Sprintf("\x1b[38;5;%d;48;5;%dm%s\x1b[0m", fg, bg, s)
}

func bold(s string) string {
	return "\x1b[1m" + s + "\x1b[0m"
}

func muted(s string) string {
	return colorFG(244, s)
}

func good(s string) string {
	return colorFG(84, s)
}

func warn(s string) string {
	return colorFG(214, s)
}

func bad(s string) string {
	return colorFG(203, s)
}

func accent(s string) string {
	return colorFG(45, s)
}

func statusBadge(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	label := strings.ToUpper(blankIf(status, "unknown"))
	switch {
	case strings.Contains(s, "error"), strings.Contains(s, "fail"):
		return colorBG(16, 203, " "+label+" ")
	case strings.Contains(s, "warn"), strings.Contains(s, "degrad"):
		return colorBG(16, 214, " "+label+" ")
	case s == "ok", s == "success", s == "completed":
		return colorBG(16, 84, " "+label+" ")
	default:
		return colorBG(16, 244, " "+label+" ")
	}
}

func pill(label string, active bool) string {
	if active {
		return colorBG(16, 45, " "+strings.ToUpper(label)+" ")
	}
	return colorBG(252, 238, " "+strings.ToUpper(label)+" ")
}

func clip(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func bar(width int, ratio float64) string {
	if width < 4 {
		width = 4
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
}
