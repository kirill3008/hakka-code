package hakkacode

import "fmt"

const (
	sgrDim    = "\033[2m"
	sgrBold   = "\033[1m"
	sgrItalic = "\033[3m"
	sgrRed    = "\033[31m"
	sgrGreen  = "\033[32m"
	sgrCyan   = "\033[36m"
	sgrReset  = "\033[0m"
)

func dim(s string) string    { return sgrDim + s + sgrReset }
func bold(s string) string   { return sgrBold + s + sgrReset }
func red(s string) string    { return sgrRed + s + sgrReset }
func green(s string) string  { return sgrGreen + s + sgrReset }
func cyan(s string) string   { return sgrCyan + s + sgrReset }

func dimf(format string, args ...any) string {
	return sgrDim + fmt.Sprintf(format, args...) + sgrReset
}
func boldf(format string, args ...any) string {
	return sgrBold + fmt.Sprintf(format, args...) + sgrReset
}
