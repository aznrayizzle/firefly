// Copyright © 2022 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlcommon

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
)

var (
	blockchainEventColumns = []string{
		"id",
		"source",
		"namespace",
		"name",
		"protocol_id",
		"listener_id",
		"output",
		"info",
		"timestamp",
		"tx_type",
		"tx_id",
		"tx_blockchain_id",
	}
	blockchainEventFilterFieldMap = map[string]string{
		"protocolid":      "protocol_id",
		"listener":        "listener_id",
		"tx.type":         "tx_type",
		"tx.id":           "tx_id",
		"tx.blockchainid": "tx_blockchain_id",
	}
)

const blockchaineventsTable = "blockchainevents"

func (s *SQLCommon) setBlockchainEventInsertValues(query sq.InsertBuilder, event *core.BlockchainEvent) sq.InsertBuilder {
	return query.Values(
		event.ID,
		event.Source,
		event.Namespace,
		event.Name,
		event.ProtocolID,
		event.Listener,
		event.Output,
		event.Info,
		event.Timestamp,
		event.TX.Type,
		event.TX.ID,
		event.TX.BlockchainID,
	)
}

func (s *SQLCommon) attemptBlockchainEventInsert(ctx context.Context, tx *txWrapper, event *core.BlockchainEvent, requestConflictEmptyResult bool) (err error) {
	_, err = s.insertTxExt(ctx, messagesTable, tx,
		s.setBlockchainEventInsertValues(sq.Insert(blockchaineventsTable).Columns(blockchainEventColumns...), event),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionBlockchainEvents, core.ChangeEventTypeCreated, event.Namespace, event.ID)
		}, requestConflictEmptyResult)
	return err
}

func (s *SQLCommon) InsertOrGetBlockchainEvent(ctx context.Context, event *core.BlockchainEvent) (existing *core.BlockchainEvent, err error) {
	ctx, tx, autoCommit, err := s.beginOrUseTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.rollbackTx(ctx, tx, autoCommit)

	opErr := s.attemptBlockchainEventInsert(ctx, tx, event, true /* we want a failure here we can progress past */)
	if opErr == nil {
		return nil, s.commitTx(ctx, tx, autoCommit)
	}

	// Do a select within the transaction to determine if the protocolID already exists
	existing, err = s.GetBlockchainEventByProtocolID(ctx, event.Namespace, event.Listener, event.ProtocolID)
	if err != nil || existing != nil {
		return existing, err
	}

	// Error was apparently not a protocolID conflict - must have been something else
	return nil, opErr
}

func (s *SQLCommon) blockchainEventResult(ctx context.Context, row *sql.Rows) (*core.BlockchainEvent, error) {
	var event core.BlockchainEvent
	err := row.Scan(
		&event.ID,
		&event.Source,
		&event.Namespace,
		&event.Name,
		&event.ProtocolID,
		&event.Listener,
		&event.Output,
		&event.Info,
		&event.Timestamp,
		&event.TX.Type,
		&event.TX.ID,
		&event.TX.BlockchainID,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, blockchaineventsTable)
	}
	return &event, nil
}

func (s *SQLCommon) getBlockchainEventPred(ctx context.Context, desc string, pred interface{}) (*core.BlockchainEvent, error) {
	rows, _, err := s.query(ctx, blockchaineventsTable,
		sq.Select(blockchainEventColumns...).
			From(blockchaineventsTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Blockchain event '%s' not found", desc)
		return nil, nil
	}

	event, err := s.blockchainEventResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (s *SQLCommon) GetBlockchainEventByID(ctx context.Context, namespace string, id *fftypes.UUID) (*core.BlockchainEvent, error) {
	return s.getBlockchainEventPred(ctx, id.String(), sq.Eq{"id": id, "namespace": namespace})
}

func (s *SQLCommon) GetBlockchainEventByProtocolID(ctx context.Context, ns string, listener *fftypes.UUID, protocolID string) (*core.BlockchainEvent, error) {
	return s.getBlockchainEventPred(ctx, protocolID, sq.Eq{
		"namespace":   ns,
		"listener_id": listener,
		"protocol_id": protocolID,
	})
}

func (s *SQLCommon) GetBlockchainEvents(ctx context.Context, namespace string, filter database.Filter) ([]*core.BlockchainEvent, *database.FilterResult, error) {

	query, fop, fi, err := s.filterSelect(ctx, "",
		sq.Select(blockchainEventColumns...).From(blockchaineventsTable),
		filter, blockchainEventFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.query(ctx, blockchaineventsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	events := []*core.BlockchainEvent{}
	for rows.Next() {
		event, err := s.blockchainEventResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, event)
	}

	return events, s.queryRes(ctx, blockchaineventsTable, tx, fop, fi), err
}
