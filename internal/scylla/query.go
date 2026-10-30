package scylla

import (
	"github.com/gocql/gocql"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

func SelectQuery(session *gocql.Session) {
	logger := logging.DefaultLogger()
	logger.Debug("selecting catalog.mutant_data")

	q := session.Query("SELECT first_name,last_name,address,picture_location FROM mutant_data")
	var firstName, lastName, address, pictureLocation string
	it := q.Iter()
	defer func() {
		if err := it.Close(); err != nil {
			logger.Errorw("select catalog.mutant_data failed", "error", err)
		}
	}()
	for it.Scan(&firstName, &lastName, &address, &pictureLocation) {
		logger.Debug("selected catalog.mutant_data row")
	}
}

func InsertQuery(session *gocql.Session, firstName, lastName, address, pictureLocation string) {
	logger := logging.DefaultLogger()
	logger.Debug("inserting catalog.mutant_data row")
	if err := session.Query("INSERT INTO mutant_data (first_name,last_name,address,picture_location) VALUES (?,?,?,?)", firstName, lastName, address, pictureLocation).Exec(); err != nil {
		logger.Errorw("insert catalog.mutant_data failed", "error", err)
	}
}

func DeleteQuery(session *gocql.Session, firstName string, lastName string) {
	logger := logging.DefaultLogger()
	logger.Debug("deleting catalog.mutant_data row")
	if err := session.Query("DELETE FROM mutant_data WHERE first_name = ? and last_name = ?", firstName, lastName).Exec(); err != nil {
		logger.Errorw("delete catalog.mutant_data failed", "error", err)
	}
}
