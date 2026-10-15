// SPDX-License-Identifier: EPL-2.0

package sysml

import "testing"

func TestEvalReportsQuantitiesAndTheirUnits(t *testing.T) {
	_, got := results(t, `package P {
    private import ISQ::*;
    private import SI::*;
    part def Vehicle {
        attribute mass : MassValue = 1800 [kg];
        attribute extra : MassValue = 200 [kg];
        attribute total = mass + extra;
        attribute within = mass <= 2000 [kg];
        attribute over = mass > 2000 [kg];
        attribute diff = 5 [kg] - 8 [kg];
        attribute half = mass / 2;
        attribute scaled = 2 * extra;
        attribute ratio = mass / extra;
        attribute real = 2.5 [kg] * 2;
        attribute same = mass == 1800 [kg];
        attribute dist : LengthValue = 3 [km];
    }
}`)
	for name, c := range map[string]struct {
		kind   ValueKind
		str    string
		number any
		symbol string
	}{
		"mass":   {KindQuantity, "1800 [kg]", int64(1800), "kg"},
		"total":  {KindQuantity, "2000 [kg]", int64(2000), "kg"},
		"diff":   {KindQuantity, "-3 [kg]", int64(-3), "kg"},
		"half":   {KindQuantity, "900 [kg]", int64(900), "kg"},
		"scaled": {KindQuantity, "400 [kg]", int64(400), "kg"},
		"real":   {KindQuantity, "5 [kg]", float64(5), "kg"},
		"dist":   {KindQuantity, "3 [km]", int64(3), "km"},
		// the units cancel
		"ratio": {KindInteger, "9", nil, ""},
		// comparisons are booleans
		"within": {KindBoolean, "true", nil, ""},
		"over":   {KindBoolean, "false", nil, ""},
		"same":   {KindBoolean, "true", nil, ""},
	} {
		r := got[name]
		if r.Status != Determined || r.Kind != c.kind || r.String() != c.str {
			t.Errorf("%s: %v (%v), want %v %s", name, r, r.Kind, c.kind, c.str)
			continue
		}
		if c.kind == KindQuantity {
			q, ok := r.Value.(Quantity)
			if !ok || q.Number != c.number || q.Symbol() != c.symbol || q.Unit == nil || q.Unit.Class != "AttributeUsage" {
				t.Errorf("%s: %+v", name, r.Value)
			}
		}
	}
	if q := got["mass"].Value.(Quantity); QualifiedName(q.Unit) != "SI::kilogram" {
		t.Errorf("the unit of mass is %s, want SI::kilogram", QualifiedName(q.Unit))
	}
}

// What the library gives no way to do is not evaluated, and is not guessed.
func TestEvalDoesNotGuessWhatItCannotConvert(t *testing.T) {
	_, got := results(t, `package P {
    private import ISQ::*;
    private import SI::*;
    part def Vehicle {
        attribute mass : MassValue = 1800 [kg];
        attribute extra : MassValue = 200 [kg];
        attribute eqd = mass == 1800 [m];
        attribute force = 5 [kg] < 1 [N];
        attribute vsnum = mass + 5;
        attribute cmpnum = mass < 5;
        attribute prod = mass * extra;
        attribute perunit = 5 / mass;
        attribute notunit = 5 [mass];
        attribute speed = 5 [m/s];
    }
}`)
	for name, reason := range map[string]string{
		// the units measure different quantities
		"eqd":     "cannot apply == to 1800 [kg] and 1800 [m]: kg and m measure different quantities",
		"force":   "cannot apply < to 5 [kg] and 1 [N]: kg and N measure different quantities",
		"vsnum":   "cannot apply + to 1800 [kg] and 5: a quantity and a number",
		"cmpnum":  "cannot apply < to 1800 [kg] and 5: a quantity and a number",
		"prod":    "cannot multiply 1800 [kg] and 200 [kg]: derived units are not supported",
		"perunit": "cannot divide 5 by 1800 [kg]: derived units are not supported",
		"notunit": "P::Vehicle::mass is not a unit",
		"speed":   "only a unit can follow a number in [ ]: derived units are not supported",
	} {
		r := got[name]
		if r.Status != NotEvaluable || r.Reason != reason || r.Value != nil {
			t.Errorf("%s: %+v, want not evaluable: %s", name, r, reason)
		}
	}
}

// Units of the same quantity convert through the conversions the library
// declares: by prefix (km to m), by convention (ft to m, h to min to s), and
// the result is in the left operand's unit.
func TestEvalConvertsBetweenUnitsOfTheSameQuantity(t *testing.T) {
	_, got := results(t, `package P {
    private import ISQ::*;
    private import SI::*;
    private import USCustomaryUnits::*;
    part def V {
        attribute sum = 3 [m] + 2 [km];
        attribute sumkm = 2 [km] + 3 [m];
        attribute small = 2 [mm] + 1 [km];
        attribute lt = 3 [m] < 2 [km];
        attribute gt = 3 [m] > 2 [km];
        attribute time = 1 [h] + 30 [min];
        attribute secs = 90 [s] < 2 [min];
        attribute same = 1 [km] == 1000 [m];
        attribute foot = 3 [ft] == 0.9144 [m];
        attribute grams = 1 [kg] == 1000 [g];
        attribute diff = 1 [kg] - 500 [g];
        attribute noteq = 1 [km] != 1001 [m];
    }
}`)
	for name, want := range map[string]string{
		"sum":   "2003 [m]",
		"sumkm": "2.003 [km]",
		"small": "1000002 [mm]",
		"lt":    "true",
		"gt":    "false",
		"time":  "1.5 [h]",
		"secs":  "true",
		"same":  "true",
		"foot":  "true", // 3 x 0.3048 is not exactly 0.9144 in binary: compared within 1e-12
		"grams": "true",
		"diff":  "0.5 [kg]",
		"noteq": "true",
	} {
		if r := got[name]; r.Status != Determined || r.String() != want {
			t.Errorf("%s: %v, want %s", name, r, want)
		}
	}
}

// A conversion factor the library gives as approximate (isExact = false) is
// carried on the quantity, and a comparison that depends on it only because
// the two are equal to within it is not decided.
func TestEvalReportsAnInexactConversion(t *testing.T) {
	_, got := results(t, `package P {
    private import ISQ::*;
    private import SI::*;
    part def V {
        attribute dalton = 2 [Da] + 1 [kg];
        attribute exact = 1 [km] + 1 [m];
        attribute larger = 1 [kg] > 1 [Da];
        attribute equal = 1 [Da] == 1.66053906660e-27 [kg];
    }
}`)
	if q, ok := got["dalton"].Value.(Quantity); !ok || !q.Inexact || q.Symbol() != "Da" {
		t.Errorf("dalton: %+v", got["dalton"])
	}
	if q, ok := got["exact"].Value.(Quantity); !ok || q.Inexact {
		t.Errorf("exact: %+v", got["exact"])
	}
	if r := got["larger"]; r.Status != Determined || r.Value != true {
		t.Errorf("larger: %v", r)
	}
	if r := got["equal"]; r.Status != NotEvaluable ||
		r.Reason != "cannot apply == to 1 [Da] and 1.6605390666e-27 [kg]: they are equal to within the precision of an inexact conversion factor" {
		t.Errorf("equal: %v", r)
	}
}
