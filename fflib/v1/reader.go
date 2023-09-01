/**
 *  Copyright 2014 Paul Querna
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package v1

import (
	"fmt"
	"io"
	"sync"
	"unicode"
	"unicode/utf16"
)

const sliceStringMask = cIJC | cNFP

var bufferPool = sync.Pool{}

func acquireBuffer() []byte {
	v := bufferPool.Get()
	if v == nil {
		return make([]byte, 128)
	}
	return v.([]byte)
}

func releaseBuffer(buffer []byte) {
	clear(buffer)
	//nolint:staticcheck
	bufferPool.Put(buffer[:0])
}

type ffReader struct {
	buffer []byte
	reader io.Reader
	head   int
	tail   int
}

func newffReader(input io.Reader) *ffReader {
	return &ffReader{
		buffer: acquireBuffer(),
		head:   0,
		reader: input,
		tail:   0,
	}
}

func (r *ffReader) Release() {
	releaseBuffer(r.buffer)
	r.buffer = nil
	r.reader = nil
}

func (r *ffReader) Slice(start, stop int) []byte {
	return r.buffer[start:stop]
}

func (r *ffReader) Pos() int {
	return r.head
}

// Reset the reader, and add new input.
func (r *ffReader) Reset(d io.Reader) {
	r.buffer = r.buffer[:0]
	r.head = 0
	r.reader = d
	r.tail = 0
}

// Calculates the Position with line and line offset,
// because this isn't counted for performance reasons,
// it will iterate the buffer from the beginning, and should
// only be used in error-paths.
func (r *ffReader) PosWithLine() (int, int) {
	currentLine := 1
	currentChar := 0

	for i := 0; i < r.head; i++ {
		c := r.buffer[i]
		currentChar++
		if c == '\n' {
			currentLine++
			currentChar = 0
		}
	}

	return currentLine, currentChar
}

func (r *ffReader) LoadMore() error {
	if r.head > 0 {
		// copy unread data to beginning of buffer.
		copy(r.buffer, r.buffer[r.head:r.tail])
		r.tail -= r.head
		r.head = 0
	}

	n, err := r.reader.Read(r.buffer[r.tail:])
	if n == 0 && err != nil {
		if err != io.EOF {
			return err
		}
	}

	r.tail += n

	return nil
}

func (r *ffReader) ReadByteNoWS() (byte, error) {
	err := r.LoadMore()
	if err != nil {
		return 0, err
	}

	if r.head >= r.tail {
		return 0, io.EOF
	}

	j := r.head

	for {
		c := r.buffer[j]
		j++

		// inline whitespace parsing gives another ~8% performance boost
		// for many kinds of nicely indented JSON.
		// ... and using a [255]bool instead of multiple ifs, gives another 2%
		/*
			if c != '\t' &&
				c != '\n' &&
				c != '\v' &&
				c != '\f' &&
				c != '\r' &&
				c != ' ' {
				r.i = j
				return c, nil
			}
		*/
		if whitespaceLookupTable[c] == false {
			r.head = j
			return c, nil
		}

		if j >= r.tail {
			return 0, io.EOF
		}
	}
}

func (r *ffReader) ReadByte() (byte, error) {
	if r.head >= r.tail {
		return 0, io.EOF
	}

	r.head++

	return r.buffer[r.head-1], nil
}

func (r *ffReader) UnreadByte() error {
	if r.head <= 0 {
		panic("ffReader.UnreadByte: at beginning of slice")
	}
	r.head--
	return nil
}

func (r *ffReader) readU4(j int) (rune, error) {

	var u4 [4]byte
	for i := 0; i < 4; i++ {
		if j >= r.tail {
			return -1, io.EOF
		}
		c := r.buffer[j]
		if byteLookupTable[c]&cVHC != 0 {
			u4[i] = c
			j++
			continue
		} else {
			// TODO(pquerna): handle errors better. layering violation.
			return -1, fmt.Errorf("lex_string_invalid_hex_char: %v %v", c, string(u4[:]))
		}
	}

	// TODO(pquerna): utf16.IsSurrogate
	rr, err := ParseUint(u4[:], 16, 64)
	if err != nil {
		return -1, err
	}
	return rune(rr), nil
}

func (r *ffReader) handleEscaped(c byte, j int, out DecodingBuffer) (int, error) {
	if j >= r.tail {
		return 0, io.EOF
	}

	c = r.buffer[j]
	j++

	if c == 'u' {
		ru, err := r.readU4(j)
		if err != nil {
			return 0, err
		}

		if utf16.IsSurrogate(ru) {
			ru2, err := r.readU4(j + 6)
			if err != nil {
				return 0, err
			}
			out.Write(r.buffer[r.head : j-2])
			r.head = j + 10
			j = r.head
			rval := utf16.DecodeRune(ru, ru2)
			if rval != unicode.ReplacementChar {
				out.WriteRune(rval)
			} else {
				return 0, fmt.Errorf("lex_string_invalid_unicode_surrogate: %v %v", ru, ru2)
			}
		} else {
			out.Write(r.buffer[r.head : j-2])
			r.head = j + 4
			j = r.head
			out.WriteRune(ru)
		}
		return j, nil
	} else if byteLookupTable[c]&cVEC == 0 {
		return 0, fmt.Errorf("lex_string_invalid_escaped_char: %v", c)
	} else {
		out.Write(r.buffer[r.head : j-2])
		r.head = j
		j = r.head

		switch c {
		case '"':
			out.WriteByte('"')
		case '\\':
			out.WriteByte('\\')
		case '/':
			out.WriteByte('/')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		}
	}

	return j, nil
}

func (r *ffReader) SliceString(out DecodingBuffer) error {
	// TODO(pquerna): string_with_escapes? de-escape here?
	j := r.head

	for {
		if j >= r.tail {
			return io.EOF
		}

		for {
			if j >= r.tail {
				err := r.LoadMore()
				if err != nil {
					return err
				}

				j = r.head
			}

			c := r.buffer[j]
			if c == '"' || byteLookupTable[c]&sliceStringMask != 0 {
				j++
				break
			}
			out.Write(r.buffer[j : j+1])
			j++

		}

		r.head = j

		return nil
		/*else if c == '\\' { // TODO handle escaped and utf16
			var err error
			j, err = r.handleEscaped(c, j, out)
			if err != nil {
				return err
			}
		} else if byteLookupTable[c]&cIJC != 0 {
			return fmt.Errorf("lex_string_invalid_json_char: %v", c)
		}*/
		continue
	}
}

// TODO(pquerna): consider combining wibth the normal byte mask.
var whitespaceLookupTable [256]bool = [256]bool{
	false, /* 0 */
	false, /* 1 */
	false, /* 2 */
	false, /* 3 */
	false, /* 4 */
	false, /* 5 */
	false, /* 6 */
	false, /* 7 */
	false, /* 8 */
	true,  /* 9 */
	true,  /* 10 */
	true,  /* 11 */
	true,  /* 12 */
	true,  /* 13 */
	false, /* 14 */
	false, /* 15 */
	false, /* 16 */
	false, /* 17 */
	false, /* 18 */
	false, /* 19 */
	false, /* 20 */
	false, /* 21 */
	false, /* 22 */
	false, /* 23 */
	false, /* 24 */
	false, /* 25 */
	false, /* 26 */
	false, /* 27 */
	false, /* 28 */
	false, /* 29 */
	false, /* 30 */
	false, /* 31 */
	true,  /* 32 */
	false, /* 33 */
	false, /* 34 */
	false, /* 35 */
	false, /* 36 */
	false, /* 37 */
	false, /* 38 */
	false, /* 39 */
	false, /* 40 */
	false, /* 41 */
	false, /* 42 */
	false, /* 43 */
	false, /* 44 */
	false, /* 45 */
	false, /* 46 */
	false, /* 47 */
	false, /* 48 */
	false, /* 49 */
	false, /* 50 */
	false, /* 51 */
	false, /* 52 */
	false, /* 53 */
	false, /* 54 */
	false, /* 55 */
	false, /* 56 */
	false, /* 57 */
	false, /* 58 */
	false, /* 59 */
	false, /* 60 */
	false, /* 61 */
	false, /* 62 */
	false, /* 63 */
	false, /* 64 */
	false, /* 65 */
	false, /* 66 */
	false, /* 67 */
	false, /* 68 */
	false, /* 69 */
	false, /* 70 */
	false, /* 71 */
	false, /* 72 */
	false, /* 73 */
	false, /* 74 */
	false, /* 75 */
	false, /* 76 */
	false, /* 77 */
	false, /* 78 */
	false, /* 79 */
	false, /* 80 */
	false, /* 81 */
	false, /* 82 */
	false, /* 83 */
	false, /* 84 */
	false, /* 85 */
	false, /* 86 */
	false, /* 87 */
	false, /* 88 */
	false, /* 89 */
	false, /* 90 */
	false, /* 91 */
	false, /* 92 */
	false, /* 93 */
	false, /* 94 */
	false, /* 95 */
	false, /* 96 */
	false, /* 97 */
	false, /* 98 */
	false, /* 99 */
	false, /* 100 */
	false, /* 101 */
	false, /* 102 */
	false, /* 103 */
	false, /* 104 */
	false, /* 105 */
	false, /* 106 */
	false, /* 107 */
	false, /* 108 */
	false, /* 109 */
	false, /* 110 */
	false, /* 111 */
	false, /* 112 */
	false, /* 113 */
	false, /* 114 */
	false, /* 115 */
	false, /* 116 */
	false, /* 117 */
	false, /* 118 */
	false, /* 119 */
	false, /* 120 */
	false, /* 121 */
	false, /* 122 */
	false, /* 123 */
	false, /* 124 */
	false, /* 125 */
	false, /* 126 */
	false, /* 127 */
	false, /* 128 */
	false, /* 129 */
	false, /* 130 */
	false, /* 131 */
	false, /* 132 */
	false, /* 133 */
	false, /* 134 */
	false, /* 135 */
	false, /* 136 */
	false, /* 137 */
	false, /* 138 */
	false, /* 139 */
	false, /* 140 */
	false, /* 141 */
	false, /* 142 */
	false, /* 143 */
	false, /* 144 */
	false, /* 145 */
	false, /* 146 */
	false, /* 147 */
	false, /* 148 */
	false, /* 149 */
	false, /* 150 */
	false, /* 151 */
	false, /* 152 */
	false, /* 153 */
	false, /* 154 */
	false, /* 155 */
	false, /* 156 */
	false, /* 157 */
	false, /* 158 */
	false, /* 159 */
	false, /* 160 */
	false, /* 161 */
	false, /* 162 */
	false, /* 163 */
	false, /* 164 */
	false, /* 165 */
	false, /* 166 */
	false, /* 167 */
	false, /* 168 */
	false, /* 169 */
	false, /* 170 */
	false, /* 171 */
	false, /* 172 */
	false, /* 173 */
	false, /* 174 */
	false, /* 175 */
	false, /* 176 */
	false, /* 177 */
	false, /* 178 */
	false, /* 179 */
	false, /* 180 */
	false, /* 181 */
	false, /* 182 */
	false, /* 183 */
	false, /* 184 */
	false, /* 185 */
	false, /* 186 */
	false, /* 187 */
	false, /* 188 */
	false, /* 189 */
	false, /* 190 */
	false, /* 191 */
	false, /* 192 */
	false, /* 193 */
	false, /* 194 */
	false, /* 195 */
	false, /* 196 */
	false, /* 197 */
	false, /* 198 */
	false, /* 199 */
	false, /* 200 */
	false, /* 201 */
	false, /* 202 */
	false, /* 203 */
	false, /* 204 */
	false, /* 205 */
	false, /* 206 */
	false, /* 207 */
	false, /* 208 */
	false, /* 209 */
	false, /* 210 */
	false, /* 211 */
	false, /* 212 */
	false, /* 213 */
	false, /* 214 */
	false, /* 215 */
	false, /* 216 */
	false, /* 217 */
	false, /* 218 */
	false, /* 219 */
	false, /* 220 */
	false, /* 221 */
	false, /* 222 */
	false, /* 223 */
	false, /* 224 */
	false, /* 225 */
	false, /* 226 */
	false, /* 227 */
	false, /* 228 */
	false, /* 229 */
	false, /* 230 */
	false, /* 231 */
	false, /* 232 */
	false, /* 233 */
	false, /* 234 */
	false, /* 235 */
	false, /* 236 */
	false, /* 237 */
	false, /* 238 */
	false, /* 239 */
	false, /* 240 */
	false, /* 241 */
	false, /* 242 */
	false, /* 243 */
	false, /* 244 */
	false, /* 245 */
	false, /* 246 */
	false, /* 247 */
	false, /* 248 */
	false, /* 249 */
	false, /* 250 */
	false, /* 251 */
	false, /* 252 */
	false, /* 253 */
	false, /* 254 */
	false, /* 255 */
}
