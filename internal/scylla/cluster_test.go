package scylla

import (
	"testing"

	"github.com/gocql/gocql"
)

func TestCluster(t *testing.T) {
	cluster := CreateCluster(gocql.Quorum, "catalog", "localhost:9042")
	session, err := gocql.NewSession(*cluster)
	if err != nil {
		t.Fatal("unable to connect to scylla", err)
	}
	defer session.Close()

	SelectQuery(session)
	InsertQuery(session, "Mike", "Tyson", "12345 Foo Lane", "http://www.facebook.com/mtyson")
	InsertQuery(session, "Alex", "Jones", "56789 Hickory St", "http://www.facebook.com/ajones")
	SelectQuery(session)
}

// more test here https://github.com/scylladb/gocql
