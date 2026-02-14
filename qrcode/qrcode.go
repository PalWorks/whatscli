/*
 *BSD 3-Clause License
 *
 *Copyright (c) 2017, Baozisoftware
 *All rights reserved.
 *
 *Redistribution and use in source and binary forms, with or without
 *modification, are permitted provided that the following conditions are met:
 *
 ** Redistributions of source code must retain the above copyright notice, this
 *  list of conditions and the following disclaimer.
 *
 ** Redistributions in binary form must reproduce the above copyright notice,
 *  this list of conditions and the following disclaimer in the documentation
 *  and/or other materials provided with the distribution.
 *
 ** Neither the name of the copyright holder nor the names of its
 *  contributors may be used to endorse or promote products derived from
 *  this software without specific prior written permission.
 *
 *THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 *AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 *IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 *DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
 *FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
 *DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 *SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
 *CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
 *OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 *OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

package qrcode

import (
	"fmt"
	"github.com/mattn/go-colorable"
	"github.com/skip2/go-qrcode"
	"io"
)

type consoleColor string
type consoleColors struct {
	NormalBlack   consoleColor
	NormalRed     consoleColor
	NormalGreen   consoleColor
	NormalYellow  consoleColor
	NormalBlue    consoleColor
	NormalMagenta consoleColor
	NormalCyan    consoleColor
	NormalWhite   consoleColor
	BrightBlack   consoleColor
	BrightRed     consoleColor
	BrightGreen   consoleColor
	BrightYellow  consoleColor
	BrightBlue    consoleColor
	BrightMagenta consoleColor
	BrightCyan    consoleColor
	BrightWhite   consoleColor
}
type qrcodeRecoveryLevel qrcode.RecoveryLevel
type qrcodeRecoveryLevels struct {
	Low     qrcodeRecoveryLevel
	Medium  qrcodeRecoveryLevel
	High    qrcodeRecoveryLevel
	Highest qrcodeRecoveryLevel
}

var (
	ConsoleColors consoleColors = consoleColors{
		NormalBlack:   "\033[38;5;0m  \033[0m",
		NormalRed:     "\033[38;5;1m  \033[0m",
		NormalGreen:   "\033[38;5;2m  \033[0m",
		NormalYellow:  "\033[38;5;3m  \033[0m",
		NormalBlue:    "\033[38;5;4m  \033[0m",
		NormalMagenta: "\033[38;5;5m  \033[0m",
		NormalCyan:    "\033[38;5;6m  \033[0m",
		NormalWhite:   "\033[38;5;7m  \033[0m",
		BrightBlack:   "\033[48;5;0m  \033[0m",
		BrightRed:     "\033[48;5;1m  \033[0m",
		BrightGreen:   "\033[48;5;2m  \033[0m",
		BrightYellow:  "\033[48;5;3m  \033[0m",
		BrightBlue:    "\033[48;5;4m  \033[0m",
		BrightMagenta: "\033[48;5;5m  \033[0m",
		BrightCyan:    "\033[48;5;6m  \033[0m",
		BrightWhite:   "\033[48;5;7m  \033[0m"}
	QRCodeRecoveryLevels = qrcodeRecoveryLevels{
		Low:     qrcodeRecoveryLevel(qrcode.Low),
		Medium:  qrcodeRecoveryLevel(qrcode.Medium),
		High:    qrcodeRecoveryLevel(qrcode.High),
		Highest: qrcodeRecoveryLevel(qrcode.Highest)}
)

type QRCodeString string

func (v *QRCodeString) Print() {
	fmt.Fprintln(outer, *v)
}

type qrcodeTerminal struct {
	front consoleColor
	back  consoleColor
	level qrcodeRecoveryLevel
}

func (v *qrcodeTerminal) Get(content interface{}) (result *QRCodeString) {
	var qr *qrcode.QRCode
	var err error
	if t, ok := content.(string); ok {
		qr, err = qrcode.New(t, qrcode.RecoveryLevel(v.level))
	} else if t, ok := content.([]byte); ok {
		qr, err = qrcode.New(string(t), qrcode.RecoveryLevel(v.level))
	}

	if qr != nil && err == nil {
		bmp := qr.Bitmap()
		result = v.renderSmall(bmp)
	}
	return
}

func (v *qrcodeTerminal) renderSmall(data [][]bool) (result *QRCodeString) {
	// Using ANSI Inverse for maximum compatibility (monochrome)
	// We rely on the terminal's default colors:
	// Default: Black BG, White FG.
	// We want: White blocks (Background) and Black blocks (Foreground/Data).

	// INVERSE (\033[7m): Swaps FG and BG.
	// Space ' ' is usually BG color.
	// Space + Inverse = FG color (White).
	// Space + Normal = BG color (Black).

	reset := "\033[0m"

	// Characters
	// ▀ (Upper Half): Upper=FG(Black), Lower=BG(White)
	// ▄ (Lower Half): Lower=FG(Black), Upper=BG(White)

	str := "" // Initialize string builder

	rows := len(data)
	if rows == 0 {
		return
	}
	cols := len(data[0])

	// Skip margin (Quiet Zone) - standard is 4 modules.
	// User requested a "Small Border" (2 modules).
	margin := 2

	// Safety check: if data provided is smaller than 2*margin, don't strip
	if rows <= 2*margin || cols <= 2*margin {
		margin = 0
	}

	for r := margin; r < rows-margin; r += 2 {
		for c := margin; c < cols-margin; c++ {
			top := data[r][c]
			bot := false
			if r+1 < rows-margin {
				bot = data[r+1][c]
			}

			// Matrix: True = Black (Module), False = White (Background)
			// But wait, standard QR code: Dark modules on Light background.
			// data[][] = true means "Dark Module".
			// data[][] = false means "Light Module".

			// We want to print:
			// True = Black (Terminal Background, usually) -> Wait, if terminal is Black BG, we want True to be Light?
			// NO. QR Code: Dark Modules on Light Background.
			// Most terminals: White FG, Black BG.
			// So we want Background (False) to be White (FG color).
			// And Modules (True) to be Black (BG color).

			// Let's stick to standard printing:
			// We want 'False' (Background) to look White (Light).
			// We want 'True' (Module) to look Black (Dark).

			// Explicit ANSI Colors (Black on Bright White)
			// FG=Black (\033[30m), BG=Bright White (\033[107m)
			// This ensures high contrast (Pure White vs Black).
			// █ (Full Block) -> Uses FG Color (Black)
			//   (Space)      -> Uses BG Color (Bright White)
			// ▀ (Upper Half) -> Top=FG(Black), Bot=BG(Bright White)
			// ▄ (Lower Half) -> Top=BG(Bright White), Bot=FG(Black)

			colorPrefix := "\033[30m\033[107m" // Black FG, Bright White BG

			if top && bot {
				// Both Black -> Full Block (FG=Black)
				str += colorPrefix + "█" + reset
			} else if !top && !bot {
				// Both White -> Space (BG=White)
				str += colorPrefix + " " + reset
			} else if top && !bot {
				// Top Black, Bot White -> Upper Half '▀' (Top=FG=Black, Bot=BG=White)
				str += colorPrefix + "▀" + reset
			} else {
				// Top White, Bot Black -> Lower Half '▄' (Bot=FG=Black, Top=BG=White)
				str += colorPrefix + "▄" + reset
			}
		}
		str += fmt.Sprintln()
	}
	obj := QRCodeString(str)
	result = &obj
	return
}

func New() *qrcodeTerminal {
	// Level Low is sufficient for cleaner/smaller QR codes
	return &qrcodeTerminal{
		level: qrcodeRecoveryLevel(qrcode.Low),
	}
}

func (_ *qrcodeTerminal) SetOutput(out io.Writer) {
	outer = out
}

var outer = colorable.NewColorableStdout()
