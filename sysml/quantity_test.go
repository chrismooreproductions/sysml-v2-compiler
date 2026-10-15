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

// What needs a conversion or a derived unit is not evaluated, and is not guessed.
func TestEvalDoesNotConvertBetweenUnits(t *testing.T) {
	_, got := results(t, `package P {
    private import ISQ::*;
    private import SI::*;
    part def Vehicle {
        attribute mass : MassValue = 1800 [kg];
        attribute extra : MassValue = 200 [kg];
        attribute mixed = 3 [m] + 2 [km];
        attribute cmp = 3 [m] < 2 [km];
        attribute eqd = mass == 1800 [m];
        attribute vsnum = mass + 5;
        attribute cmpnum = mass < 5;
        attribute prod = mass * extra;
        attribute perunit = 5 / mass;
        attribute notunit = 5 [mass];
        attribute speed = 5 [m/s];
    }
}`)
	for name, reason := range map[string]string{
		"mixed":   "cannot apply + to 3 [m] and 2 [km]: converting between units is not supported",
		"cmp":     "cannot apply < to 3 [m] and 2 [km]: converting between units is not supported",
		"eqd":     "cannot apply == to 1800 [kg] and 1800 [m]: converting between units is not supported",
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
