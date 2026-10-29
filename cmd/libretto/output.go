package main

import (
	"scanlibretto/internal/libreta"
	"scanlibretto/internal/outjson"
)

// Il JSON in uscita e' costruito da internal/outjson (condiviso con libretto2); qui restano i nomi usati da main e dai test.
type (
	output  = outjson.Output
	warning = outjson.Warning
)

func toOutput(res *libreta.Result) output { return outjson.From(res) }
func toWarning(text string) warning       { return outjson.ToWarning(text) }
func fieldName(internal string) string    { return outjson.FieldName(internal) }
func correction(s string) string          { return outjson.Correction(s) }
