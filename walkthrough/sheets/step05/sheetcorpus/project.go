// walkthrough/sheets/step05/sheetcorpus/project.go
package sheetcorpus

import "github.com/sdboyer/hypercase/decl"

var Project = decl.Project{
	UpdateVar: "SHEETS_UPDATE_GOLDEN",
	Positions: positions,
}

var Circular = decl.Property{
	Name:        "circular",
	Description: "Some cell's formula refers back to itself, directly or through other cells.",
}

// doc:begin properties
var Malformed = decl.Property{
	Name:        "malformed",
	Description: "Some cell holds neither a number nor a formula.",
}

var Dangling = decl.Property{
	Name:        "dangling",
	Description: "Some formula refers to a cell the sheet doesn't have.",
}

// doc:end properties
