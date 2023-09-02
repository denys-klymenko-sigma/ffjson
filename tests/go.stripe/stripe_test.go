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

package goser

import (
	"bytes"
	"encoding/json"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/sanity-io/litter"
	"github.com/stretchr/testify/assert"

	"github.com/denys-klymenko-sigma/ffjson/ffjson"
	base "github.com/denys-klymenko-sigma/ffjson/tests/go.stripe/base"
	ff "github.com/denys-klymenko-sigma/ffjson/tests/go.stripe/ff"
)

func TestRoundTrip(t *testing.T) {
	customer := ff.NewCustomer()

	buf1, err := ffjson.Marshal(&customer)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	decoder := ffjson.NewDecoder()

	var customerTripped1 ff.Customer

	err = decoder.DecodeFast(bytes.NewReader(buf1), &customerTripped1)
	if err != nil {
		print(string(buf1))
		t.Fatalf("Unmarshal: %v", err)
	}

	assert.Equal(t, litter.Sdump(*customer), litter.Sdump(customerTripped1))

	var customerTripped2 ff.Customer

	buf, cust := getBaseData(t)

	err = decoder.DecodeFast(bytes.NewReader(buf), &customerTripped2)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	assert.Equal(t, litter.Sdump(*cust), litter.Sdump(customerTripped2))
}

func BenchmarkMarshalJSON(b *testing.B) {
	cust := base.NewCustomer()

	buf, err := json.Marshal(&cust)
	if err != nil {
		b.Fatalf("Marshal: %v", err)
	}
	b.SetBytes(int64(len(buf)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(&cust)
		if err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}

func BenchmarkFFMarshalJSON(b *testing.B) {
	cust := ff.NewCustomer()

	buf, err := cust.MarshalJSON()
	if err != nil {
		b.Fatalf("Marshal: %v", err)
	}
	b.SetBytes(int64(len(buf)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cust.MarshalJSON()
		if err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}

type fatalF interface {
	Fatalf(format string, args ...interface{})
}

func getBaseData(b fatalF) ([]byte, *base.Customer) {
	cust := base.NewCustomer()
	buf, err := json.MarshalIndent(&cust, "", "    ")
	if err != nil {
		b.Fatalf("Marshal: %v", err)
	}
	return buf, cust
}

func BenchmarkFFUnmarshalJSON(b *testing.B) {
	b.Run("json", func(b *testing.B) {
		rec := base.Customer{}
		buf, _ := getBaseData(b)
		b.SetBytes(int64(len(buf)))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := json.Unmarshal(buf, &rec)
			if err != nil {
				b.Fatalf("Marshal: %v", err)
			}
		}
	})

	b.Run("jsoniter", func(b *testing.B) {
		rec := base.Customer{}
		buf, _ := getBaseData(b)
		b.SetBytes(int64(len(buf)))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := jsoniter.Unmarshal(buf, &rec)
			if err != nil {
				b.Fatalf("Marshal: %v", err)
			}
		}
	})

	b.Run("ffjson", func(b *testing.B) {
		rec := ff.Customer{}
		buf, _ := getBaseData(b)
		b.SetBytes(int64(len(buf)))

		decoder := ffjson.NewDecoder()

		reader := bytes.NewReader(buf)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := decoder.DecodeFast(reader, &rec)
			if err != nil {
				b.Fatalf("UnmarshalJSON: %v", err)
			}
			reader.Reset(buf)
		}
	})
}
