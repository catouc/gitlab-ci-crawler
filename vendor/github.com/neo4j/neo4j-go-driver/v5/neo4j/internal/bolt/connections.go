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

package bolt

import (
	"bufio"
	"context"
	"io"
	"net"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j/db"
	idb "github.com/neo4j/neo4j-go-driver/v5/neo4j/internal/db"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/internal/errorutil"
)

// DefaultReadBufferSize specifies the default size (in bytes) of the buffer used for reading data from the network connection.
const DefaultReadBufferSize = 8192

func bufferedConnection(conn net.Conn, readBufferSize int) io.ReadWriteCloser {
	var reader io.Reader
	if readBufferSize > 0 {
		reader = bufio.NewReaderSize(conn, readBufferSize)
	} else {
		reader = conn
	}

	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{
		Reader: reader,
		Writer: conn,
		Closer: conn,
	}
}

type ConnectionErrorListener interface {
	OnNeo4jError(context.Context, idb.Connection, *db.Neo4jError) error
	OnIoError(context.Context, idb.Connection, error)
	OnDialError(context.Context, string, error)
}

func handleTerminatedContextError(err error, connection io.Closer) error {
	if !contextTerminatedErr(err) {
		return nil
	}
	closeErr := connection.Close()
	if closeErr == nil {
		return nil
	}
	return errorutil.CombineErrors(err, closeErr)
}

func contextTerminatedErr(err error) bool {
	switch err.(type) {
	case *errorutil.ConnectionWriteTimeout:
		return true
	case *errorutil.ConnectionReadTimeout:
		return true
	case *errorutil.ConnectionWriteCanceled:
		return true
	case *errorutil.ConnectionReadCanceled:
		return true
	}
	return false
}
