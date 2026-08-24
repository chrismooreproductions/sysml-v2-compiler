package sysml_test

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

// environmentModel and combatantsModel are standalone packages, each
// imported by battlefieldModel below rather than nested inside it -- see
// the metamodel package's own TestFromASTWithImports_Battlefield for the
// cross-model resolution this setup is meant to exercise.
const environmentModel = `package Environment {
    part def Geology;
    part def Terrain;
    part def Climate;

    part geology : Geology;
    part terrain : Terrain;
    part climate : Climate;
}`

const combatantsModel = `package Combatants {
    part def Combatant;

    part friendlyCombatants : Combatant[*];
    part enemyCombatants : Combatant[*];
}`

const battlefieldModel = `package Battlefield {
    import Environment;
    import Combatants;

    part terrain : Environment::Terrain;
    part squad : Combatants::Combatant[*];
}`
