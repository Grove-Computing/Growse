package ui

import "fmt"

func selectionSummary(label string, total, selected int) string {
	if total <= 0 {
		return label + " 0件"
	}
	if selected < 0 || selected >= total {
		return fmt.Sprintf("%s %d件、未選択", label, total)
	}
	return fmt.Sprintf("%s %d件、選択 %d / %d", label, total, selected+1, total)
}
