// walkthrough/sheets/step04/sheetcorpus/project.go
package sheetcorpus

import "github.com/sdboyer/hypercase/decl"

var Project = decl.Project{
	UpdateVar: "SHEETS_UPDATE_GOLDEN",
}

// doc:begin properties
var Circular = decl.Property{
	Name:        "circular",
	Description: "Some cell's formula refers back to itself, directly or through other cells.",
}

// doc:end properties
