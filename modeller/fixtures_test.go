package modeller_test

// vehicleModel is the canonical fixture shared by the lexer and parser
// tests. Its exact layout (spaces vs. the tab before the final `}`) is
// load-bearing for the byte-offset assertions in TestModelLex, so edit
// with care and re-derive those offsets if it changes.
const vehicleModel = `package Vehicle {
    part def Engine;

    part def Car {
        part engine : Engine;
    }
	}`
