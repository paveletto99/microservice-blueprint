package scylla

import (
	"os"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
)

func TestCluster(t *testing.T) {
	t.Parallel()

	hosts := []string{"127.0.0.1:9042", "127.0.0.2:9042"}
	cluster := CreateCluster(gocql.Quorum, "catalog", hosts...)
	if cluster.Keyspace != "catalog" || cluster.Consistency != gocql.Quorum {
		t.Errorf("unexpected keyspace or consistency: %q, %v", cluster.Keyspace, cluster.Consistency)
	}
	if len(cluster.Hosts) != len(hosts) {
		t.Fatalf("host count = %d, want %d", len(cluster.Hosts), len(hosts))
	}
	for i, host := range hosts {
		if cluster.Hosts[i] != host {
			t.Errorf("host %d = %q, want %q", i, cluster.Hosts[i], host)
		}
	}
	if cluster.Timeout != 5*time.Second {
		t.Errorf("timeout = %s, want 5s", cluster.Timeout)
	}
	retry, ok := cluster.RetryPolicy.(*gocql.ExponentialBackoffRetryPolicy)
	if !ok {
		t.Fatalf("retry policy = %T, want exponential backoff", cluster.RetryPolicy)
	}
	if retry.Min != time.Second || retry.Max != 10*time.Second || retry.NumRetries != 5 {
		t.Errorf("unexpected retry settings: %+v", retry)
	}
	if cluster.PoolConfig.HostSelectionPolicy == nil {
		t.Error("host selection policy is missing")
	}
}

func TestClusterIntegration(t *testing.T) {
	host := os.Getenv("SCYLLA_TEST_HOST")
	keyspace := os.Getenv("SCYLLA_TEST_KEYSPACE")
	if host == "" && keyspace == "" {
		t.Skip("set SCYLLA_TEST_HOST and SCYLLA_TEST_KEYSPACE to run against an isolated test database")
	}
	if host == "" || keyspace == "" {
		t.Fatal("both SCYLLA_TEST_HOST and SCYLLA_TEST_KEYSPACE are required")
	}
	cluster := CreateCluster(gocql.Quorum, keyspace, host)
	session, err := gocql.NewSession(*cluster)
	if err != nil {
		t.Fatal("unable to connect to scylla", err)
	}
	t.Cleanup(session.Close)

	firstName, lastName := "blueprint-"+uuid.NewString(), "integration"
	t.Cleanup(func() {
		if err := session.Query("DELETE FROM mutant_data WHERE first_name = ? and last_name = ?", firstName, lastName).Exec(); err != nil {
			t.Error("unable to clean up test row", err)
		}
	})
	SelectQuery(session)
	InsertQuery(session, firstName, lastName, "test-address", "https://example.invalid/fixture")
	var address, pictureLocation string
	query := "SELECT address,picture_location FROM mutant_data WHERE first_name = ? and last_name = ?"
	if err := session.Query(query, firstName, lastName).Scan(&address, &pictureLocation); err != nil {
		t.Fatal("unable to read inserted row", err)
	}
	if address != "test-address" || pictureLocation != "https://example.invalid/fixture" {
		t.Fatal("inserted row did not match fixture")
	}
	SelectQuery(session)
	DeleteQuery(session, firstName, lastName)
	if err := session.Query(query, firstName, lastName).Scan(&address, &pictureLocation); err != gocql.ErrNotFound {
		t.Fatalf("read after deletion returned %v, want row not found", err)
	}
}

// more test here https://github.com/scylladb/gocql
