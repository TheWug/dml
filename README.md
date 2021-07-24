# dml
Database Mapping Layer - a toy project in golang designed to make database plumbing a little bit easier.

##Usage
This thing is designed to make reading objects from a database in golang easier. If you're familiar with how the json package chooses which fields to place data into when unmarshalling data, the approach here will be familiar to you. In short, this project enables the following patterns:

```
type Foo struct {
	MyInt    int    `dml:"my_int"`
	MyString string `dml:"my_string"`
}

row := DB.Query("select my_int, my_string from somewhere")
// or rows, err := dml.X(DB.QueryRows("select my_int, my_string from somewhere"))
var foo Foo

dml.Scan(row, &foo)
// or dml.Scan(rows, &foo)

sql.Row and sql.Rows are both supported, but Row has limitations: in particular, Row does not expose ColumnTypes, and this limits dml's ability to automatically map between results (they are assumed to all be required, in order). If you need per-field processing, use QueryRow instead.

This is a very early project and it is likely to change a lot. Don't use it yet.

##Running tests
`{ go test . -coverprofile=p.out; go tool cover -func=p.out; } && go tool cover -html=p.out -o coverage.html`
