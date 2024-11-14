package scylla

import (
	"fmt"

	"github.com/gocql/gocql"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

func SelectQuery(session *gocql.Session) {
	logger := logging.DefaultLogger()
	logger.Info("🕵️")

	q := session.Query("SELECT first_name,last_name,address,picture_location FROM mutant_data")
	var firstName, lastName, address, pictureLocation string
	it := q.Iter()
	defer func() {
		if err := it.Close(); err != nil {
			logger.Warn("select catalog.mutant", err.Error(), err)
		}
	}()
	for it.Scan(&firstName, &lastName, &address, &pictureLocation) {
		logger.Info("\t" + firstName + " " + lastName + ", " + address + ", " + pictureLocation)
		fmt.Println(firstName)
	}
}

func InsertQuery(session *gocql.Session, firstName, lastName, address, pictureLocation string) {
	logger := logging.DefaultLogger()
	logger.Info("Inserting " + firstName + "......")
	if err := session.Query("INSERT INTO mutant_data (first_name,last_name,address,picture_location) VALUES (?,?,?,?)", firstName, lastName, address, pictureLocation).Exec(); err != nil {
		logger.Error("insert catalog.mutant_data", err.Error(), err)
	}
}

func DeleteQuery(session *gocql.Session, firstName string, lastName string) {
	logger := logging.DefaultLogger()
	logger.Info("Deleting " + firstName + "......")
	if err := session.Query("DELETE FROM mutant_data WHERE first_name = ? and last_name = ?", firstName, lastName).Exec(); err != nil {
		logger.Error("delete catalog.mutant_data", err.Error(), err)
	}
}
