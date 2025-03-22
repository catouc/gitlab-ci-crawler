//go:build internal_testkit && internal_time_mock

/*
 * Copyright (c) "Neo4j"
 * Neo4j Sweden AB [https://neo4j.com]
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package neo4j

import (
	"context"
	"fmt"
	idb "github.com/neo4j/neo4j-go-driver/v5/neo4j/internal/db"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/internal/errorutil"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/internal/router"
	itime "github.com/neo4j/neo4j-go-driver/v5/neo4j/internal/time"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/log"
)

type RoutingTable = idb.RoutingTable

func ForceRoutingTableUpdate(d DriverWithContext, database string, bookmarks []string, logger log.BoltLogger) error {
	driver := d.(*driverWithContext)
	ctx := context.Background()
	driver.router.Invalidate(database)
	getBookmarks := func(context.Context) ([]string, error) {
		return bookmarks, nil
	}
	auth := &idb.ReAuthToken{
		Manager:     driver.auth,
		FromSession: false,
		ForceReAuth: false,
	}
	dbSelection := idb.DatabaseSelection{Name: database}
	_, err := driver.router.GetOrUpdateReaders(ctx, getBookmarks, dbSelection, auth, logger, func(db string) {
		if dbSelection.Name == "" {
			dbSelection.Name = db
		}
	})
	if err != nil {
		return errorutil.WrapError(err)
	}
	_, err = driver.router.GetOrUpdateWriters(ctx, getBookmarks, dbSelection, auth, logger, nil)
	return errorutil.WrapError(err)
}

func RegisterDnsResolver(d DriverWithContext, hook func(address string) []string) {
	d.(*driverWithContext).connector.TestKitDnsResolver = hook
}

func GetRoutingTable(d DriverWithContext, database string) (*RoutingTable, error) {
	driver := d.(*driverWithContext)
	router, ok := driver.router.(*router.Router)
	if !ok {
		return nil, fmt.Errorf("GetRoutingTable is only supported for direct drivers")
	}
	table := router.GetTable(database)
	return table, nil
}

var Now = itime.Now
var FreezeTime = itime.FreezeTime
var TickTime = itime.TickTime
var UnfreezeTime = itime.UnfreezeTime
