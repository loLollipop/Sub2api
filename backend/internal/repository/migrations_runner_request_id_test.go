package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func requestIDSearchMigrationFS(t *testing.T) fstest.MapFS {
	t.Helper()
	body, err := migrations.FS.ReadFile(requestIDSearchIndexesMigration)
	require.NoError(t, err)
	return fstest.MapFS{requestIDSearchIndexesMigration: {Data: body}}
}

func TestRequestIDSearchMigrationSplitsIntoExactlyFourExecutableStatements(t *testing.T) {
	fsys := requestIDSearchMigrationFS(t)
	statements := splitSQLStatements(string(fsys[requestIDSearchIndexesMigration].Data))
	var executable []string
	for _, statement := range statements {
		if clean := stripSQLLineComment(strings.TrimSpace(statement)); clean != "" {
			executable = append(executable, clean)
		}
	}
	require.Len(t, executable, len(requestIDSearchIndexes))
	for i, statement := range executable {
		require.True(t, strings.HasPrefix(statement, "CREATE INDEX CONCURRENTLY IF NOT EXISTS "+requestIDSearchIndexes[i].name+"\n"), statement)
	}
}

func TestRequestIDSearchMigrationPrerequisiteFailureStopsBeforeIndexes(t *testing.T) {
	for _, queryFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing or inaccessible", true: "catalog error"}[queryFailure], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			prepareMigrationsBootstrapExpectations(mock)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
				WithArgs(requestIDSearchIndexesMigration).WillReturnError(sql.ErrNoRows)
			check := mock.ExpectQuery(`(?s)SELECT EXISTS.*pg_catalog.pg_extension.*pg_opclass_is_visible.*has_schema_privilege`)
			if queryFailure {
				check.WillReturnError(errors.New("catalog permission denied"))
			} else {
				check.WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			}
			// No DROP/CREATE INDEX or migration completion write is allowed.
			mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
				WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
			err = applyMigrationsFS(context.Background(), db, requestIDSearchMigrationFS(t))
			require.ErrorContains(t, err, "prepare migration "+requestIDSearchIndexesMigration)
			if queryFailure {
				require.ErrorContains(t, err, "catalog permission denied")
			} else {
				require.ErrorContains(t, err, "pg_trgm")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRequestIDSearchMigrationRetriesInvalidConcurrentIndexes(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid indexes retained", true: "invalid indexes rebuilt"}[invalid], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			prepareMigrationsBootstrapExpectations(mock)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
				WithArgs(requestIDSearchIndexesMigration).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`(?s)SELECT EXISTS.*pg_catalog.pg_extension.*pg_opclass_is_visible.*has_schema_privilege`).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			for _, index := range requestIDSearchIndexes {
				mock.ExpectQuery(`(?s)SELECT.*pg_catalog.to_regclass\(\$2\)`).WithArgs(index.name, index.table, index.column).
					WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}).AddRow("public", true, !invalid, true, true))
				if invalid {
					mock.ExpectExec(regexp.QuoteMeta(`DROP INDEX CONCURRENTLY IF EXISTS "public"."` + index.name + `"`)).
						WillReturnResult(sqlmock.NewResult(0, 0))
				}
			}
			for _, statement := range splitSQLStatements(string(requestIDSearchMigrationFS(t)[requestIDSearchIndexesMigration].Data)) {
				if stripSQLLineComment(statement) == "" {
					continue
				}
				mock.ExpectExec("(?s)^" + regexp.QuoteMeta(strings.TrimSpace(statement)) + "$").
					WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO schema_migrations (filename, checksum) VALUES ($1, $2)")).
				WithArgs(requestIDSearchIndexesMigration, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
				WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, applyMigrationsFS(context.Background(), db, requestIDSearchMigrationFS(t)))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRequestIDSearchMigrationIndexFailureDoesNotRecordCompletion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
		WithArgs(requestIDSearchIndexesMigration).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT EXISTS.*pg_catalog.pg_extension.*pg_opclass_is_visible.*has_schema_privilege`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	for _, index := range requestIDSearchIndexes {
		mock.ExpectQuery(`(?s)SELECT.*pg_catalog.to_regclass\(\$2\)`).WithArgs(index.name, index.table, index.column).
			WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}).AddRow("public", false, false, false, false))
	}
	mock.ExpectExec("CREATE INDEX CONCURRENTLY IF NOT EXISTS " + requestIDSearchIndexes[0].name).
		WillReturnError(errors.New("index build canceled"))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	err = applyMigrationsFS(context.Background(), db, requestIDSearchMigrationFS(t))
	require.ErrorContains(t, err, "index build canceled")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestIDSearchMigrationExtensionFailureRollsBackAndStops(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	const prerequisite = "263_require_request_id_search_trgm.sql"
	body, err := migrations.FS.ReadFile(prerequisite)
	require.NoError(t, err)
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
		WithArgs(prerequisite).WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(string(body))).
		WillReturnError(errors.New("permission denied to create extension pg_trgm"))
	mock.ExpectRollback()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	err = applyMigrationsFS(context.Background(), db, fstest.MapFS{prerequisite: {Data: body}})
	require.ErrorContains(t, err, "permission denied to create extension pg_trgm")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestIDSearchMigrationInvalidIndexDropFailureStops(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	prepareMigrationsBootstrapExpectations(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
		WithArgs(requestIDSearchIndexesMigration).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT EXISTS.*pg_catalog.pg_extension.*pg_opclass_is_visible.*has_schema_privilege`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`(?s)SELECT.*pg_catalog.to_regclass\(\$2\)`).WithArgs(requestIDSearchIndexes[0].name, "usage_logs", "request_id").
		WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}).AddRow("public", true, false, true, true))
	mock.ExpectExec(regexp.QuoteMeta(`DROP INDEX CONCURRENTLY IF EXISTS "public"."` + requestIDSearchIndexes[0].name + `"`)).
		WillReturnError(errors.New("index drop canceled"))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
		WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
	err = applyMigrationsFS(context.Background(), db, requestIDSearchMigrationFS(t))
	require.ErrorContains(t, err, "index drop canceled")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestIDSearchMigrationInvalidIndexUsesResolvedTableSchema(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	mock.ExpectQuery(`(?s)SELECT EXISTS.*pg_catalog.pg_extension`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	for _, index := range requestIDSearchIndexes {
		mock.ExpectQuery(`(?s)SELECT.*pg_catalog.to_regclass\(\$2\)`).WithArgs(index.name, index.table, index.column).
			WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}).AddRow(`tenant"logs`, true, false, true, true))
		mock.ExpectExec(regexp.QuoteMeta(`DROP INDEX CONCURRENTLY IF EXISTS "tenant""logs"."` + index.name + `"`)).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	require.NoError(t, prepareNonTransactionalMigration(context.Background(), conn, requestIDSearchIndexesMigration))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequestIDSearchMigrationConflictingRelationStopsBeforeDrop(t *testing.T) {
	for _, valid := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		prepareMigrationsBootstrapExpectations(mock)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT checksum FROM schema_migrations WHERE filename = $1")).
			WithArgs(requestIDSearchIndexesMigration).WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery(`(?s)SELECT EXISTS.*pg_catalog.pg_extension`).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery(`(?s)SELECT.*i.indrelid = tbl.oid.*idx.relnamespace = ns.oid.*pg_catalog.to_regclass\(\$2\)`).
			WithArgs(requestIDSearchIndexes[0].name, "usage_logs", "request_id").
			WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}).AddRow("tenant_logs", true, valid, false, false))
		mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock($1)")).
			WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
		err = applyMigrationsFS(context.Background(), db, requestIDSearchMigrationFS(t))
		require.ErrorContains(t, err, "conflicts with a different relation")
		require.NoError(t, mock.ExpectationsWereMet(), "no DROP, CREATE or completion write is authorized for a conflicting relation")
	}
}

func TestRequestIDSearchMigrationTargetResolutionFailureStops(t *testing.T) {
	for _, missing := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		query := mock.ExpectQuery(`(?s)SELECT.*pg_catalog.to_regclass\(\$2\)`).WithArgs(requestIDSearchIndexes[0].name, "usage_logs", "request_id")
		if missing {
			query.WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}))
		} else {
			query.WillReturnError(errors.New("catalog denied"))
		}
		err = dropInvalidRequestIDSearchIndex(context.Background(), conn, "usage_logs", requestIDSearchIndexes[0].name, "request_id")
		require.ErrorContains(t, err, "resolve request-ID search index")
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestRequestIDSearchMigrationWrongIndexDefinitionStopsBeforeDrop(t *testing.T) {
	for _, valid := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		conn, err := db.Conn(context.Background())
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		// Independent definition requirements, not a regex of production SQL.
		query := `(?s)SELECT.*i.indisvalid AND i.indisready.*am.amname = 'gin'.*i.indnkeyatts = 1 AND i.indnatts = 1.*i.indexprs IS NULL AND i.indpred IS NULL.*i.indkey\[0\] = att.attnum AND i.indclass\[0\] = opc.oid.*i.indcollation\[0\] = att.attcollation.*pg_attribute.*att.attname = \$3.*pg_extension.*ext.extname = 'pg_trgm'.*pg_opclass.*opc.opcnamespace = ext.extnamespace.*opc.opcname = 'gin_trgm_ops'.*pg_catalog.to_regclass\(\$2\)`
		mock.ExpectQuery(query).WithArgs(requestIDSearchIndexes[0].name, "usage_logs", "request_id").
			WillReturnRows(sqlmock.NewRows([]string{"schema", "exists", "valid", "target", "definition"}).AddRow("public", true, valid, true, false))
		err = dropInvalidRequestIDSearchIndex(context.Background(), conn, "usage_logs", requestIDSearchIndexes[0].name, "request_id")
		require.ErrorContains(t, err, "incompatible definition")
		require.NoError(t, mock.ExpectationsWereMet(), "an unrelated or incorrectly defined index must not be dropped or accepted")
	}
}
