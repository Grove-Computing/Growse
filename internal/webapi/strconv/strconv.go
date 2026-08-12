// Package strconv はWebGoスクリプト向けの文字列変換APIを提供する。
package strconv

import stdstrconv "strconv"

// Itoa は整数を10進数の文字列へ変換する。
func Itoa(value int) string {
	return stdstrconv.Itoa(value)
}
