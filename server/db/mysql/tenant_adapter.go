//go:build mysql
// +build mysql

package mysql

import (
	"database/sql"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/tinode/chat/server/db/common"
	"github.com/tinode/chat/server/store"
	t "github.com/tinode/chat/server/store/types"
)

func (*adapter) TenantBusinessReady() bool { return true }

func validTenant(id t.TenantID) error {
	if !id.IsValid() {
		return t.ErrMalformed
	}
	return nil
}

func tenantCacheKey(id t.TenantID, key string) (string, error) {
	if key == "" || strings.Contains(key, "%") {
		return "", t.ErrMalformed
	}
	return "tenant:" + strconv.FormatInt(int64(id), 10) + ":" + key, nil
}

func decodeUID(uid t.Uid) int64 {
	return store.DecodeUid(uid)
}

func encodeUserID(id int64) string {
	return store.EncodeUid(id).String()
}

func scanTenantUser(user *t.User) {
	user.SetUid(common.EncodeUidString(user.Id))
	user.Public = common.FromJSON(user.Public)
	user.Trusted = common.FromJSON(user.Trusted)
}

func scanTenantTopic(topic *t.Topic) {
	topic.Owner = common.EncodeUidString(topic.Owner).String()
	topic.Public = common.FromJSON(topic.Public)
	topic.Trusted = common.FromJSON(topic.Trusted)
}

func (a *adapter) TenantUserCreate(id t.TenantID, user *t.User) error {
	if err := validTenant(id); err != nil || user == nil || user.TenantID != id {
		return t.ErrMalformed
	}
	return a.UserCreate(user)
}

func (a *adapter) TenantUserGet(id t.TenantID, uid t.Uid) (*t.User, error) {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	var user t.User
	err := a.db.GetContext(ctx, &user, "SELECT * FROM users WHERE tenant_id=? AND id=? AND state!=?",
		id, decodeUID(uid), t.StateDeleted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	scanTenantUser(&user)
	return &user, nil
}

func (a *adapter) TenantUserGetAll(id t.TenantID, uids ...t.Uid) ([]t.User, error) {
	if err := validTenant(id); err != nil {
		return nil, err
	}
	if len(uids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(uids))
	for _, uid := range uids {
		if !uid.IsZero() {
			args = append(args, decodeUID(uid))
		}
	}
	if len(args) == 0 {
		return nil, nil
	}
	q, params, _ := sqlx.In("SELECT * FROM users WHERE tenant_id=? AND id IN (?) AND state!=?", id, args, t.StateDeleted)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, a.db.Rebind(q), params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []t.User
	for rows.Next() {
		var user t.User
		if err = rows.StructScan(&user); err != nil {
			return nil, err
		}
		scanTenantUser(&user)
		users = append(users, user)
	}
	return users, rows.Err()
}

func (a *adapter) TenantUserDelete(id t.TenantID, uid t.Uid, hard bool) error {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return t.ErrMalformed
	}
	decoded := decodeUID(uid)
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var ownTopics []string
	q := "SELECT name FROM topics WHERE tenant_id=? AND owner=?"
	args := []any{id, decoded}
	if !hard {
		q += " AND state!=?"
		args = append(args, t.StateDeleted)
	}
	if err = tx.Select(&ownTopics, q, args...); err != nil {
		return err
	}
	var ownAll []any
	for _, name := range ownTopics {
		ownAll = append(ownAll, name)
		if chn := t.GrpToChn(name); chn != "" {
			ownAll = append(ownAll, chn)
		}
	}

	now := t.TimeNow()
	if hard {
		if _, err = tx.Exec("DELETE FROM devices WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
			return err
		}
		if err = tenantSubsDelForUser(tx, id, decoded, true); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM dellog WHERE tenant_id=? AND deletedfor=?", id, decoded); err != nil {
			return err
		}
		if len(ownTopics) > 0 {
			if _, err = tx.Exec("DELETE dl FROM dellog AS dl JOIN topics AS tp ON tp.tenant_id=dl.tenant_id AND tp.name=dl.topic WHERE tp.tenant_id=? AND tp.owner=?", id, decoded); err != nil {
				return err
			}
			if _, err = tx.Exec("DELETE m FROM messages AS m JOIN topics AS tp ON tp.tenant_id=m.tenant_id AND tp.name=m.topic WHERE tp.tenant_id=? AND tp.owner=?", id, decoded); err != nil {
				return err
			}
			sqlq, params, _ := sqlx.In("DELETE FROM subscriptions WHERE tenant_id=? AND topic IN (?)", id, ownAll)
			if _, err = tx.Exec(tx.Rebind(sqlq), params...); err != nil {
				return err
			}
			if _, err = tx.Exec("DELETE tt FROM topictags AS tt JOIN topics AS tp ON tp.tenant_id=tt.tenant_id AND tp.name=tt.topic WHERE tp.tenant_id=? AND tp.owner=?", id, decoded); err != nil {
				return err
			}
			if _, err = tx.Exec("DELETE FROM topics WHERE tenant_id=? AND owner=?", id, decoded); err != nil {
				return err
			}
		}
		if _, err = tx.Exec("DELETE FROM auth WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM credentials WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM usertags WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM users WHERE tenant_id=? AND id=?", id, decoded); err != nil {
			return err
		}
	} else {
		if err = tenantSubsDelForUser(tx, id, decoded, false); err != nil {
			return err
		}
		if len(ownAll) > 0 {
			sqlq, params, _ := sqlx.In("UPDATE subscriptions SET updatedat=?,deletedat=? WHERE tenant_id=? AND topic IN (?)", now, now, id, ownAll)
			if _, err = tx.Exec(tx.Rebind(sqlq), params...); err != nil {
				return err
			}
		}
		if _, err = tx.Exec("UPDATE topics SET updatedat=?,touchedat=?,state=?,stateat=? WHERE tenant_id=? AND owner=?",
			now, now, t.StateDeleted, now, id, decoded); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE topics AS tp JOIN subscriptions AS s ON tp.tenant_id=s.tenant_id AND tp.name=s.topic "+
			"SET tp.updatedat=?,tp.touchedat=?,tp.state=?,tp.stateat=? WHERE tp.tenant_id=? AND tp.owner=0 AND s.userid=? AND tp.name LIKE 'p2p%'",
			now, now, t.StateDeleted, now, id, decoded); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE subscriptions AS s_one JOIN subscriptions AS s_two ON s_one.tenant_id=s_two.tenant_id AND s_one.topic=s_two.topic "+
			"SET s_two.updatedat=?,s_two.deletedat=? WHERE s_one.tenant_id=? AND s_one.userid=? AND s_one.topic LIKE 'p2p%'",
			now, now, id, decoded); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE users SET updatedat=?,state=?,stateat=? WHERE tenant_id=? AND id=?",
			now, t.StateDeleted, now, id, decoded); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *adapter) tenantTopicStateForUser(tx *sqlx.Tx, tenantID t.TenantID, decodedUID int64, now time.Time, state any) error {
	objState, ok := state.(t.ObjState)
	if !ok {
		return t.ErrMalformed
	}
	if now.IsZero() {
		now = t.TimeNow()
	}
	if _, err := tx.Exec("UPDATE topics SET state=?,stateat=? WHERE tenant_id=? AND owner=? AND state!=?",
		objState, now, tenantID, decodedUID, t.StateDeleted); err != nil {
		return err
	}
	_, err := tx.Exec("UPDATE topics AS tp JOIN subscriptions AS s ON tp.tenant_id=s.tenant_id AND tp.name=s.topic "+
		"SET tp.state=?,tp.stateat=? WHERE tp.tenant_id=? AND tp.owner=0 AND s.userid=? AND tp.state!=?",
		objState, now, tenantID, decodedUID, t.StateDeleted)
	return err
}

func (a *adapter) TenantUserUpdate(id t.TenantID, uid t.Uid, update map[string]any) error {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	decoded := decodeUID(uid)
	cols, args := common.UpdateByMap(update)
	args = append(args, id, decoded)
	res, err := tx.Exec("UPDATE users SET "+strings.Join(cols, ",")+" WHERE tenant_id=? AND id=?", args...)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count == 0 {
		return t.ErrNotFound
	}
	if state, ok := update["State"]; ok {
		now, _ := update["StateAt"].(time.Time)
		if err = a.tenantTopicStateForUser(tx, id, decoded, now, state); err != nil {
			return err
		}
	}
	if tags := common.ExtractTags(update); tags != nil {
		if _, err = tx.Exec("DELETE FROM usertags WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
			return err
		}
		if err = addTenantTags(tx, id, "usertags", "userid", decoded, tags, false); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *adapter) TenantUserUpdateTags(id t.TenantID, uid t.Uid, add, remove, reset []string) ([]string, error) {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	decoded := decodeUID(uid)
	if reset != nil {
		if _, err = tx.Exec("DELETE FROM usertags WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
			return nil, err
		}
		add, remove = reset, nil
	}
	if err = addTenantTags(tx, id, "usertags", "userid", decoded, add, reset == nil); err != nil {
		return nil, err
	}
	if len(remove) > 0 {
		q, params, _ := sqlx.In("DELETE FROM usertags WHERE tenant_id=? AND userid=? AND tag IN (?)", id, decoded, remove)
		if _, err = tx.Exec(tx.Rebind(q), params...); err != nil {
			return nil, err
		}
	}
	var allTags []string
	if err = tx.Select(&allTags, "SELECT tag FROM usertags WHERE tenant_id=? AND userid=?", id, decoded); err != nil {
		return nil, err
	}
	if _, err = tx.Exec("UPDATE users SET tags=? WHERE tenant_id=? AND id=?", t.StringSlice(allTags), id, decoded); err != nil {
		return nil, err
	}
	return allTags, tx.Commit()
}

func (a *adapter) TenantUserUnreadCount(id t.TenantID, uids ...t.Uid) (map[t.Uid]int, error) {
	if err := validTenant(id); err != nil {
		return nil, err
	}
	counts := make(map[t.Uid]int, len(uids))
	var decoded []any
	for _, uid := range uids {
		counts[uid] = 0
		if !uid.IsZero() {
			decoded = append(decoded, decodeUID(uid))
		}
	}
	if len(decoded) == 0 {
		return counts, nil
	}
	q, args, _ := sqlx.In("SELECT s.userid,SUM(tp.seqid)-SUM(s.readseqid) AS unreadcount "+
		"FROM topics AS tp JOIN subscriptions AS s ON tp.tenant_id=s.tenant_id AND tp.name=s.topic "+
		"WHERE s.tenant_id=? AND s.userid IN (?) AND s.deletedat IS NULL AND tp.state!=? "+
		"AND INSTR(s.modewant,'R')>0 AND INSTR(s.modegiven,'R')>0 GROUP BY s.userid", id, decoded, t.StateDeleted)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, a.db.Rebind(q), args...)
	if err != nil {
		return counts, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var unread int
		if err = rows.Scan(&uid, &unread); err != nil {
			return counts, err
		}
		counts[store.EncodeUid(uid)] = unread
	}
	return counts, rows.Err()
}

func (a *adapter) TenantUserGetUnvalidated(id t.TenantID, before time.Time, limit int) ([]t.Uid, error) {
	if err := validTenant(id); err != nil {
		return nil, err
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx,
		"SELECT u.id,IFNULL(SUM(c.done),0) AS total FROM users AS u "+
			"LEFT JOIN credentials AS c ON u.tenant_id=c.tenant_id AND u.id=c.userid "+
			"WHERE u.tenant_id=? AND u.lastseen IS NULL AND u.updatedat<? "+
			"GROUP BY u.id,u.updatedat HAVING total=0 ORDER BY u.updatedat ASC LIMIT ?", id, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var uids []t.Uid
	for rows.Next() {
		var userID int64
		var unused int
		if err = rows.Scan(&userID, &unused); err != nil {
			return nil, err
		}
		uids = append(uids, store.EncodeUid(userID))
	}
	return uids, rows.Err()
}

func tenantTopicCreate(tx *sqlx.Tx, topic *t.Topic) error {
	if topic == nil || topic.TenantID.IsZero() {
		return t.ErrMalformed
	}
	_, err := tx.Exec("INSERT INTO topics(tenant_id,createdat,updatedat,touchedat,state,name,usebt,owner,access,public,trusted,tags,aux) "+
		"VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)",
		topic.TenantID, topic.CreatedAt, topic.UpdatedAt, topic.TouchedAt, topic.State, topic.Id, topic.UseBt,
		decodeUID(t.ParseUid(topic.Owner)), topic.Access, common.ToJSON(topic.Public), common.ToJSON(topic.Trusted),
		topic.Tags, common.ToJSON(topic.Aux))
	if err != nil {
		return err
	}
	return addTenantTags(tx, topic.TenantID, "topictags", "topic", topic.Id, topic.Tags, false)
}

func (a *adapter) TenantTopicCreate(id t.TenantID, topic *t.Topic) error {
	if err := validTenant(id); err != nil || topic == nil || topic.TenantID != id {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = tenantTopicCreate(tx, topic); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *adapter) TenantTopicCreateP2P(id t.TenantID, one, two *t.Subscription) error {
	if err := validTenant(id); err != nil || one == nil || two == nil || one.TenantID != id || two.TenantID != id {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = createSubscription(tx, one, false); err != nil {
		return err
	}
	if err = createSubscription(tx, two, true); err != nil {
		return err
	}
	topic := &t.Topic{TenantID: id, ObjHeader: t.ObjHeader{Id: one.Topic}}
	topic.ObjHeader.MergeTimes(&one.ObjHeader)
	topic.TouchedAt = one.GetTouchedAt()
	if err = tenantTopicCreate(tx, topic); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *adapter) TenantTopicGet(id t.TenantID, topic string) (*t.Topic, error) {
	if err := validTenant(id); err != nil || topic == "" {
		return nil, t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	var top t.Topic
	err := a.db.GetContext(ctx, &top,
		"SELECT tenant_id,createdat,updatedat,state,stateat,touchedat,name AS id,usebt,access,owner,seqid,delid,subcnt,public,trusted,tags,aux "+
			"FROM topics WHERE tenant_id=? AND name=?", id, topic)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.GetTopicCat(topic) == t.TopicCatGrp {
		var subCnt int
		if err = a.db.GetContext(ctx, &subCnt,
			"SELECT COUNT(*) FROM subscriptions WHERE tenant_id=? AND topic IN (?,?) AND deletedat IS NULL",
			id, topic, t.GrpToChn(topic)); err != nil {
			return nil, err
		}
		if subCnt != top.SubCnt {
			top.SubCnt = subCnt
			if _, err = a.db.ExecContext(ctx, "UPDATE topics SET subcnt=? WHERE tenant_id=? AND name=?", subCnt, id, topic); err != nil {
				return nil, err
			}
		}
	}
	scanTenantTopic(&top)
	return &top, nil
}

func (a *adapter) TenantTopicsForUser(id t.TenantID, uid t.Uid, keepDeleted bool, opts *t.QueryOpt) ([]t.Subscription, error) {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	q := `SELECT tenant_id,createdat,updatedat,deletedat,topic,delid,recvseqid,
		readseqid,modewant,modegiven,private FROM subscriptions WHERE tenant_id=? AND userid=?`
	args := []any{id, decodeUID(uid)}
	if !keepDeleted {
		q += " AND deletedat IS NULL"
	}
	limit := 0
	ims := time.Time{}
	if opts != nil {
		if opts.Topic != "" {
			q += " AND topic=?"
			args = append(args, opts.Topic)
		}
		if opts.IfModifiedSince == nil {
			limit = a.maxResults
			if opts.Limit > 0 && opts.Limit < limit {
				limit = opts.Limit
			}
		} else {
			ims = *opts.IfModifiedSince
		}
	} else {
		limit = a.maxResults
	}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	join := map[string]t.Subscription{}
	var topq, usrq []any
	for rows.Next() {
		var sub t.Subscription
		if err = rows.StructScan(&sub); err != nil {
			rows.Close()
			return nil, err
		}
		tname := sub.Topic
		sub.User = uid.String()
		switch t.GetTopicCat(tname) {
		case t.TopicCatMe, t.TopicCatFnd:
			continue
		case t.TopicCatP2P:
			uid1, uid2, _ := t.ParseP2P(tname)
			if uid1 == uid {
				usrq = append(usrq, decodeUID(uid2))
				sub.SetWith(uid2.UserId())
			} else {
				usrq = append(usrq, decodeUID(uid1))
				sub.SetWith(uid1.UserId())
			}
		case t.TopicCatGrp:
			tname = t.ChnToGrp(tname)
		}
		topq = append(topq, tname)
		sub.Private = common.FromJSON(sub.Private)
		join[tname] = sub
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(join) == 0 {
		return nil, err
	}
	if len(topq) > 0 {
		q = "SELECT tenant_id,updatedat,state,touchedat,name AS id,usebt,access,seqid,delid,subcnt,public,trusted FROM topics WHERE tenant_id=? AND name IN (?)"
		q, args, _ = sqlx.In(q, id, topq)
		if !keepDeleted {
			q += " AND state!=?"
			args = append(args, t.StateDeleted)
		}
		if !ims.IsZero() {
			q += " AND touchedat>?"
			args = append(args, ims)
			if limit > 0 && limit < len(topq) {
				q += " ORDER BY touchedat LIMIT ?"
				args = append(args, limit)
			}
		}
		rows, err = a.db.QueryxContext(ctx, a.db.Rebind(q), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var top t.Topic
			if err = rows.StructScan(&top); err != nil {
				rows.Close()
				return nil, err
			}
			sub := join[top.Id]
			sub.UpdatedAt = common.SelectLatestTime(sub.UpdatedAt, top.UpdatedAt)
			sub.SetState(top.State)
			sub.SetTouchedAt(top.TouchedAt)
			sub.SetSeqId(top.SeqId)
			if t.GetTopicCat(sub.Topic) == t.TopicCatGrp {
				sub.SetSubCnt(top.SubCnt)
				sub.SetPublic(common.FromJSON(top.Public))
				sub.SetTrusted(common.FromJSON(top.Trusted))
			}
			join[top.Id] = sub
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if len(usrq) > 0 {
		q = "SELECT tenant_id,id,updatedat,state,access,lastseen,useragent,public,trusted FROM users WHERE tenant_id=? AND id IN (?)"
		q, args, _ = sqlx.In(q, id, usrq)
		if !keepDeleted {
			q += " AND state!=?"
			args = append(args, t.StateDeleted)
		}
		rows, err = a.db.QueryxContext(ctx, a.db.Rebind(q), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var user t.User
			if err = rows.StructScan(&user); err != nil {
				rows.Close()
				return nil, err
			}
			joinOn := uid.P2PName(common.EncodeUidString(user.Id))
			if sub, ok := join[joinOn]; ok {
				sub.UpdatedAt = common.SelectLatestTime(sub.UpdatedAt, user.UpdatedAt)
				sub.SetState(user.State)
				sub.SetPublic(common.FromJSON(user.Public))
				sub.SetTrusted(common.FromJSON(user.Trusted))
				sub.SetDefaultAccess(user.Access.Auth, user.Access.Anon)
				sub.SetLastSeenAndUA(user.LastSeen, user.UserAgent)
				join[joinOn] = sub
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	subs := make([]t.Subscription, 0, len(join))
	for _, sub := range join {
		subs = append(subs, sub)
	}
	return common.SelectEarliestUpdatedSubs(subs, opts, a.maxResults), nil
}

func (a *adapter) TenantUsersForTopic(id t.TenantID, topic string, keepDeleted bool, opts *t.QueryOpt) ([]t.Subscription, error) {
	if err := validTenant(id); err != nil || topic == "" {
		return nil, t.ErrMalformed
	}
	tcat := t.GetTopicCat(topic)
	q := `SELECT s.tenant_id,s.createdat,s.updatedat,s.deletedat,s.userid,s.topic,s.delid,s.recvseqid,
		s.readseqid,s.modewant,s.modegiven,u.public,u.trusted,u.lastseen,u.useragent,s.private
		FROM subscriptions AS s JOIN users AS u ON s.tenant_id=u.tenant_id AND s.userid=u.id
		WHERE s.tenant_id=? AND s.topic=?`
	args := []any{id, topic}
	if !keepDeleted {
		q += " AND u.state!=?"
		args = append(args, t.StateDeleted)
		if tcat != t.TopicCatP2P {
			q += " AND s.deletedat IS NULL"
		}
	}
	limit := a.maxResults
	var oneUser t.Uid
	if opts != nil {
		if !opts.User.IsZero() {
			if tcat != t.TopicCatP2P {
				q += " AND s.userid=?"
				args = append(args, decodeUID(opts.User))
			}
			oneUser = opts.User
		}
		if opts.Limit > 0 && opts.Limit < limit {
			limit = opts.Limit
		}
	}
	q += " LIMIT ?"
	args = append(args, limit)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []t.Subscription
	var userAgent string
	for rows.Next() {
		var sub t.Subscription
		var lastSeen sql.NullTime
		var public, trusted any
		if err = rows.Scan(&sub.TenantID, &sub.CreatedAt, &sub.UpdatedAt, &sub.DeletedAt,
			&sub.User, &sub.Topic, &sub.DelId, &sub.RecvSeqId, &sub.ReadSeqId, &sub.ModeWant, &sub.ModeGiven,
			&public, &trusted, &lastSeen, &userAgent, &sub.Private); err != nil {
			return nil, err
		}
		sub.User = common.EncodeUidString(sub.User).String()
		sub.Private = common.FromJSON(sub.Private)
		sub.SetPublic(common.FromJSON(public))
		sub.SetTrusted(common.FromJSON(trusted))
		sub.SetLastSeenAndUA(&lastSeen.Time, userAgent)
		subs = append(subs, sub)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if tcat == t.TopicCatP2P && len(subs) > 0 {
		if len(subs) == 1 {
			subs[0].SetPublic(nil)
			subs[0].SetTrusted(nil)
			subs[0].SetLastSeenAndUA(nil, "")
		} else {
			pub := subs[0].GetPublic()
			subs[0].SetPublic(subs[1].GetPublic())
			subs[1].SetPublic(pub)
			trusted := subs[0].GetTrusted()
			subs[0].SetTrusted(subs[1].GetTrusted())
			subs[1].SetTrusted(trusted)
			lastSeen := subs[0].GetLastSeen()
			ua := subs[0].GetUserAgent()
			subs[0].SetLastSeenAndUA(subs[1].GetLastSeen(), subs[1].GetUserAgent())
			subs[1].SetLastSeenAndUA(lastSeen, ua)
		}
		if !keepDeleted || !oneUser.IsZero() {
			filtered := subs[:0]
			for i := range subs {
				if (subs[i].DeletedAt != nil && !keepDeleted) || (!oneUser.IsZero() && subs[i].Uid() != oneUser) {
					continue
				}
				filtered = append(filtered, subs[i])
			}
			subs = filtered
		}
	}
	return subs, nil
}

func (a *adapter) tenantTopicNames(query string, args ...any) ([]string, error) {
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (a *adapter) TenantOwnTopics(id t.TenantID, uid t.Uid) ([]string, error) {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	return a.tenantTopicNames("SELECT name FROM topics WHERE tenant_id=? AND owner=? AND state!=?",
		id, decodeUID(uid), t.StateDeleted)
}

func (a *adapter) TenantChannelsForUser(id t.TenantID, uid t.Uid) ([]string, error) {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	return a.tenantTopicNames("SELECT topic FROM subscriptions WHERE tenant_id=? AND userid=? AND topic LIKE 'chn%' "+
		"AND INSTR(modewant,'P')>0 AND INSTR(modegiven,'P')>0 AND deletedat IS NULL", id, decodeUID(uid))
}

func (a *adapter) TenantTopicShare(id t.TenantID, topic string, subs []*t.Subscription) error {
	if err := validTenant(id); err != nil {
		return err
	}
	for _, sub := range subs {
		if sub == nil || sub.TenantID != id {
			return t.ErrMalformed
		}
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	for _, sub := range subs {
		if err = createSubscription(tx, sub, true); err != nil {
			return err
		}
	}
	if topic != "" {
		if _, err = tx.Exec("UPDATE topics SET subcnt=subcnt+? WHERE tenant_id=? AND name=?", len(subs), id, topic); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func tenantMessageDeleteList(tx *sqlx.Tx, tenantID t.TenantID, topic string, toDel *t.DelMessage) error {
	if toDel == nil {
		if _, err := tx.Exec("DELETE FROM dellog WHERE tenant_id=? AND topic=?", tenantID, topic); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM messages WHERE tenant_id=? AND topic=?", tenantID, topic)
		return err
	}
	delRanges := toDel.SeqIdRanges
	if toDel.DeletedFor == "" {
		where := "m.tenant_id=? AND m.topic=?"
		args := []any{tenantID, topic}
		if len(delRanges) > 0 {
			rSQL, rArgs := common.RangesToSql(delRanges)
			where += " AND m.seqid " + rSQL
			args = append(args, rArgs...)
		}
		where += " AND m.deletedat IS NULL"
		if newerThan := toDel.GetNewerThan(); newerThan != nil {
			where += " AND m.createdat>?"
			args = append(args, newerThan)
		}
		var seqIDs []int
		if err := tx.Select(&seqIDs, "SELECT seqid FROM messages AS m WHERE "+where, args...); err != nil {
			return err
		}
		if len(seqIDs) == 0 {
			return nil
		}
		sort.Ints(seqIDs)
		delRanges = t.SliceToRanges(seqIDs)
		rSQL, rArgs := common.RangesToSql(delRanges)
		where = "m.tenant_id=? AND m.topic=? AND m.seqid " + rSQL
		args = append([]any{tenantID, topic}, rArgs...)
		if _, err := tx.Exec("DELETE fml.* FROM filemsglinks AS fml INNER JOIN messages AS m ON m.tenant_id=fml.tenant_id AND m.id=fml.msgid WHERE "+where, args...); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE messages AS m SET m.deletedat=?,m.delid=?,m.`from`=0,m.head=NULL,m.content=NULL WHERE "+
			where, append([]any{t.TimeNow(), toDel.DelId}, args...)...); err != nil {
			return err
		}
	}
	stmt, err := tx.Prepare("INSERT INTO dellog(tenant_id,topic,deletedfor,delid,low,hi) VALUES(?,?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	forUser := common.DecodeUidString(toDel.DeletedFor)
	for _, rng := range delRanges {
		if rng.Hi == 0 {
			rng.Hi = rng.Low + 1
		}
		if _, err = stmt.Exec(tenantID, topic, forUser, toDel.DelId, rng.Low, rng.Hi); err != nil {
			return err
		}
	}
	return nil
}

func (a *adapter) TenantTopicDelete(id t.TenantID, topic string, isChan, hard bool) error {
	if err := validTenant(id); err != nil || topic == "" {
		return t.ErrMalformed
	}
	topics := []any{topic}
	if isChan {
		topics = append(topics, t.GrpToChn(topic))
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if hard {
		q, args, _ := sqlx.In("DELETE FROM subscriptions WHERE tenant_id=? AND topic IN (?)", id, topics)
		if _, err = tx.Exec(tx.Rebind(q), args...); err != nil {
			return err
		}
		if err = tenantMessageDeleteList(tx, id, topic, nil); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM topictags WHERE tenant_id=? AND topic=?", id, topic); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM topics WHERE tenant_id=? AND name=?", id, topic); err != nil {
			return err
		}
	} else {
		now := t.TimeNow()
		q, args, _ := sqlx.In("UPDATE subscriptions SET updatedat=?,deletedat=? WHERE tenant_id=? AND topic IN (?)", now, now, id, topics)
		if _, err = tx.Exec(tx.Rebind(q), args...); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE topics SET updatedat=?,touchedat=?,state=?,stateat=? WHERE tenant_id=? AND name=?",
			now, now, t.StateDeleted, now, id, topic); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *adapter) TenantTopicUpdateOnMessage(id t.TenantID, topic string, msg *t.Message) error {
	if err := validTenant(id); err != nil || msg == nil || msg.TenantID != id {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	_, err := a.db.ExecContext(ctx, "UPDATE topics SET seqid=?,touchedat=? WHERE tenant_id=? AND name=?",
		msg.SeqId, msg.CreatedAt, id, topic)
	return err
}

func (a *adapter) TenantTopicUpdateSubCnt(id t.TenantID, topic string) error {
	if err := validTenant(id); err != nil || topic == "" {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	_, err := a.db.ExecContext(ctx,
		"UPDATE topics SET subcnt=(SELECT COUNT(*) FROM subscriptions WHERE tenant_id=? AND topic IN (?,?) AND deletedat IS NULL) WHERE tenant_id=? AND name=?",
		id, topic, t.GrpToChn(topic), id, topic)
	return err
}

func (a *adapter) TenantTopicUpdate(id t.TenantID, topic string, update map[string]any) error {
	if err := validTenant(id); err != nil || topic == "" {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if update["TouchedAt"] == nil && update["UpdatedAt"] != nil {
		update["TouchedAt"] = update["UpdatedAt"]
	}
	cols, args := common.UpdateByMap(update)
	args = append(args, id, topic)
	res, err := tx.Exec("UPDATE topics SET "+strings.Join(cols, ",")+" WHERE tenant_id=? AND name=?", args...)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count == 0 {
		return t.ErrNotFound
	}
	if tags := common.ExtractTags(update); tags != nil {
		if _, err = tx.Exec("DELETE FROM topictags WHERE tenant_id=? AND topic=?", id, topic); err != nil {
			return err
		}
		if err = addTenantTags(tx, id, "topictags", "topic", topic, tags, false); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *adapter) TenantTopicOwnerChange(id t.TenantID, topic string, owner t.Uid) error {
	if err := validTenant(id); err != nil || topic == "" {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	_, err := a.db.ExecContext(ctx, "UPDATE topics SET owner=? WHERE tenant_id=? AND name=?", decodeUID(owner), id, topic)
	return err
}

func (a *adapter) TenantSubscriptionGet(id t.TenantID, topic string, uid t.Uid, keepDeleted bool) (*t.Subscription, error) {
	if err := validTenant(id); err != nil || topic == "" || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	query := `SELECT tenant_id,createdat,updatedat,deletedat,userid AS user,topic,delid,recvseqid,
		readseqid,modewant,modegiven,private FROM subscriptions WHERE tenant_id=? AND topic=? AND userid=?`
	if !keepDeleted {
		query += " AND deletedat IS NULL"
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	var sub t.Subscription
	err := a.db.GetContext(ctx, &sub, query, id, topic, decodeUID(uid))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sub.User = uid.String()
	sub.Private = common.FromJSON(sub.Private)
	return &sub, nil
}

func (a *adapter) TenantSubsForUser(id t.TenantID, uid t.Uid) ([]t.Subscription, error) {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return nil, t.ErrMalformed
	}
	rows, err := a.db.Queryx(`SELECT tenant_id,createdat,updatedat,deletedat,userid AS user,topic,delid,recvseqid,
		readseqid,modewant,modegiven FROM subscriptions WHERE tenant_id=? AND userid=? AND deletedat IS NULL`,
		id, decodeUID(uid))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []t.Subscription
	for rows.Next() {
		var sub t.Subscription
		if err = rows.StructScan(&sub); err != nil {
			return nil, err
		}
		sub.User = uid.String()
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (a *adapter) TenantSubsForTopic(id t.TenantID, topic string, keepDeleted bool, opts *t.QueryOpt) ([]t.Subscription, error) {
	if err := validTenant(id); err != nil || topic == "" {
		return nil, t.ErrMalformed
	}
	q := `SELECT tenant_id,createdat,updatedat,deletedat,userid AS user,topic,delid,recvseqid,
		readseqid,modewant,modegiven,private FROM subscriptions WHERE tenant_id=? AND topic=?`
	args := []any{id, topic}
	if !keepDeleted {
		q += " AND deletedat IS NULL"
	}
	limit := a.maxResults
	if opts != nil {
		if !opts.User.IsZero() {
			q += " AND userid=?"
			args = append(args, decodeUID(opts.User))
		}
		if opts.Limit > 0 && opts.Limit < limit {
			limit = opts.Limit
		}
	}
	q += " LIMIT ?"
	args = append(args, limit)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []t.Subscription
	for rows.Next() {
		var sub t.Subscription
		if err = rows.StructScan(&sub); err != nil {
			return nil, err
		}
		sub.User = common.EncodeUidString(sub.User).String()
		sub.Private = common.FromJSON(sub.Private)
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (a *adapter) TenantSubsUpdate(id t.TenantID, topic string, uid t.Uid, update map[string]any) error {
	if err := validTenant(id); err != nil || topic == "" {
		return t.ErrMalformed
	}
	cols, args := common.UpdateByMap(update)
	q := "UPDATE subscriptions SET " + strings.Join(cols, ",") + " WHERE tenant_id=? AND topic=?"
	args = append(args, id, topic)
	if !uid.IsZero() {
		q += " AND userid=?"
		args = append(args, decodeUID(uid))
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	_, err := a.db.ExecContext(ctx, q, args...)
	return err
}

func (a *adapter) TenantSubsDelete(id t.TenantID, topic string, uid t.Uid) error {
	if err := validTenant(id); err != nil || topic == "" || uid.IsZero() {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	now := t.TimeNow()
	decoded := decodeUID(uid)
	res, err := tx.Exec("UPDATE subscriptions SET updatedat=?,deletedat=? WHERE tenant_id=? AND topic=? AND userid=? AND deletedat IS NULL",
		now, now, id, topic, decoded)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count == 0 {
		return t.ErrNotFound
	}
	if !t.IsChannel(topic) {
		if _, err = tx.Exec("DELETE FROM dellog WHERE tenant_id=? AND topic=? AND deletedfor=?", id, topic, decoded); err != nil {
			return err
		}
	}
	if t.GetTopicCat(topic) == t.TopicCatGrp {
		if _, err = tx.Exec("UPDATE topics SET subcnt=subcnt-1 WHERE tenant_id=? AND name=?", id, t.ChnToGrp(topic)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func tenantSubsDelForUser(tx *sqlx.Tx, tenantID t.TenantID, decodedUID int64, hard bool) error {
	rows, err := tx.Query("SELECT topic FROM subscriptions WHERE tenant_id=? AND userid=? AND deletedat IS NULL", tenantID, decodedUID)
	if err != nil {
		return err
	}
	var topics []any
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		if t.IsChannel(name) {
			name = t.ChnToGrp(name)
		}
		topics = append(topics, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(topics) > 0 {
		q, args, _ := sqlx.In("UPDATE topics SET subcnt=subcnt-1 WHERE tenant_id=? AND name IN (?)", tenantID, topics)
		if _, err = tx.Exec(tx.Rebind(q), args...); err != nil {
			return err
		}
	}
	if hard {
		_, err = tx.Exec("DELETE FROM subscriptions WHERE tenant_id=? AND userid=?", tenantID, decodedUID)
	} else {
		now := t.TimeNow()
		_, err = tx.Exec("UPDATE subscriptions SET updatedat=?,deletedat=? WHERE tenant_id=? AND userid=? AND deletedat IS NULL",
			now, now, tenantID, decodedUID)
	}
	return err
}

func (a *adapter) TenantFind(id t.TenantID, caller, promoPrefix string, req [][]string, opt []string, activeOnly bool) ([]t.Subscription, error) {
	if err := validTenant(id); err != nil {
		return nil, err
	}
	allReq := t.FlattenDoubleSlice(req)
	allTags := append(append([]string{}, allReq...), opt...)
	if len(allTags) == 0 {
		return nil, nil
	}
	args := []any{id}
	stateConstraint := ""
	if activeOnly {
		args = append(args, t.StateOK)
		stateConstraint = "u.state=? AND "
	}
	index := make(map[string]struct{})
	for _, tag := range allTags {
		args = append(args, tag)
		index[tag] = struct{}{}
	}
	matcher := "COUNT(*)"
	if promoPrefix != "" {
		matcher = "SUM(CASE WHEN LOCATE('" + promoPrefix + "', tg.tag)=1 THEN 20 ELSE 1 END)"
	}
	query := "SELECT u.id,u.createdat,u.updatedat,0,u.access,0 AS subcnt,u.public,u.trusted,u.tags," + matcher + " AS matches " +
		"FROM users AS u JOIN usertags AS tg ON tg.tenant_id=u.tenant_id AND tg.userid=u.id " +
		"WHERE u.tenant_id=? AND " + stateConstraint + "tg.tag IN (?" + strings.Repeat(",?", len(allTags)-1) + ") " +
		"GROUP BY u.id,u.createdat,u.updatedat,u.access,u.public,u.trusted,u.tags "
	if len(allReq) > 0 {
		q, addArgs := common.DisjunctionSql(req, "tg.tag")
		query += q
		args = append(args, addArgs...)
	}
	query += "UNION ALL "
	args = append(args, id)
	if activeOnly {
		args = append(args, t.StateOK)
		stateConstraint = "t.state=? AND "
	} else {
		stateConstraint = ""
	}
	for _, tag := range allTags {
		args = append(args, tag)
	}
	query += "SELECT t.name AS topic,t.createdat,t.updatedat,t.usebt,t.access,t.subcnt,t.public,t.trusted,t.tags," + matcher + " AS matches " +
		"FROM topics AS t JOIN topictags AS tg ON t.tenant_id=tg.tenant_id AND t.name=tg.topic " +
		"WHERE t.tenant_id=? AND " + stateConstraint + "tg.tag IN (?" + strings.Repeat(",?", len(allTags)-1) + ") " +
		"GROUP BY t.name,t.createdat,t.updatedat,t.usebt,t.access,t.subcnt,t.public,t.trusted,t.tags "
	if len(allReq) > 0 {
		q, addArgs := common.DisjunctionSql(req, "tg.tag")
		query += q
		args = append(args, addArgs...)
	}
	query += "ORDER BY matches DESC, subcnt DESC LIMIT ?"
	args = append(args, a.maxResults)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []t.Subscription
	for rows.Next() {
		var sub t.Subscription
		var public, trusted any
		var access t.DefaultAccess
		var subcnt, ignored int
		var setTags t.StringSlice
		var isChan bool
		if err = rows.Scan(&sub.Topic, &sub.CreatedAt, &sub.UpdatedAt, &isChan, &access, &subcnt,
			&public, &trusted, &setTags, &ignored); err != nil {
			return nil, err
		}
		if parsed, err := strconv.ParseInt(sub.Topic, 10, 64); err == nil {
			sub.Topic = store.EncodeUid(parsed).UserId()
			if sub.Topic == caller {
				continue
			}
		}
		if isChan {
			sub.Topic = t.GrpToChn(sub.Topic)
		}
		sub.TenantID = id
		sub.SetSubCnt(subcnt)
		sub.SetPublic(common.FromJSON(public))
		sub.SetTrusted(common.FromJSON(trusted))
		sub.SetDefaultAccess(access.Auth, access.Anon)
		sub.ModeGiven = t.ModeUnset
		sub.ModeWant = t.ModeUnset
		sub.Private = common.FilterFoundTags(setTags, index)
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (a *adapter) TenantFindOne(id t.TenantID, tag string) (string, error) {
	if err := validTenant(id); err != nil || tag == "" {
		return "", t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx,
		"SELECT tp.name AS topic FROM topics AS tp LEFT JOIN topictags AS tt ON tp.tenant_id=tt.tenant_id AND tp.name=tt.topic "+
			"WHERE tp.tenant_id=? AND tt.tag=? UNION ALL "+
			"SELECT u.id AS topic FROM users AS u LEFT JOIN usertags AS ut ON u.tenant_id=ut.tenant_id AND ut.userid=u.id "+
			"WHERE u.tenant_id=? AND ut.tag=? LIMIT 1", id, tag, id, tag)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var found string
	if rows.Next() {
		if err = rows.Scan(&found); err != nil {
			return "", err
		}
		if parsed, err := strconv.ParseInt(found, 10, 64); err == nil {
			found = store.EncodeUid(parsed).UserId()
		}
	}
	return found, rows.Err()
}

func (a *adapter) TenantMessageSave(id t.TenantID, msg *t.Message) error {
	if err := validTenant(id); err != nil || msg == nil || msg.TenantID != id {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	res, err := a.db.ExecContext(ctx,
		"INSERT INTO messages(tenant_id,createdat,updatedat,seqid,topic,`from`,head,content) VALUES(?,?,?,?,?,?,?,?)",
		id, msg.CreatedAt, msg.UpdatedAt, msg.SeqId, msg.Topic, decodeUID(t.ParseUid(msg.From)), msg.Head, common.ToJSON(msg.Content))
	if err == nil {
		lastID, _ := res.LastInsertId()
		msg.SetUid(t.Uid(lastID))
	}
	return err
}

func (a *adapter) TenantMessageGetAll(id t.TenantID, topic string, uid t.Uid, opts *t.QueryOpt) ([]t.Message, error) {
	if err := validTenant(id); err != nil || topic == "" {
		return nil, t.ErrMalformed
	}
	limit := a.maxMessageResults
	args := []any{id, topic, id, decodeUID(uid)}
	seq := ""
	if opts != nil {
		seq = "AND m.seqid "
		if len(opts.IdRanges) > 0 {
			constr, addArgs := common.RangesToSql(opts.IdRanges)
			seq += constr
			args = append(args, addArgs...)
		} else {
			seq += "BETWEEN ? AND ?"
			if opts.Since > 0 {
				args = append(args, opts.Since)
			} else {
				args = append(args, 0)
			}
			if opts.Before > 1 {
				args = append(args, opts.Before-1)
			} else {
				args = append(args, 1<<31-1)
			}
		}
		if opts.Limit > 0 && opts.Limit < limit {
			limit = opts.Limit
		}
	}
	args = append(args, limit)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx,
		"SELECT m.tenant_id,m.createdat,m.updatedat,m.deletedat,m.delid,m.seqid,m.topic,m.`from`,m.head,m.content "+
			"FROM messages AS m LEFT JOIN dellog AS d ON d.tenant_id=m.tenant_id AND d.topic=m.topic "+
			"AND m.seqid BETWEEN d.low AND d.hi-1 AND d.deletedfor=? WHERE m.tenant_id=? AND m.delid=0 AND m.topic=? "+
			seq+" AND d.deletedfor IS NULL ORDER BY m.seqid DESC LIMIT ?",
		append([]any{decodeUID(uid), id, topic}, args[4:]...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs := make([]t.Message, 0, limit)
	for rows.Next() {
		var msg t.Message
		if err = rows.StructScan(&msg); err != nil {
			return nil, err
		}
		msg.From = common.EncodeUidString(msg.From).String()
		msg.Content = common.FromJSON(msg.Content)
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

func (a *adapter) TenantMessageDeleteList(id t.TenantID, topic string, del *t.DelMessage) error {
	if err := validTenant(id); err != nil || topic == "" || (del != nil && del.TenantID != id) {
		return t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = tenantMessageDeleteList(tx, id, topic, del); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *adapter) TenantMessageGetDeleted(id t.TenantID, topic string, uid t.Uid, opts *t.QueryOpt) ([]t.DelMessage, error) {
	if err := validTenant(id); err != nil || topic == "" {
		return nil, t.ErrMalformed
	}
	limit, lower, upper := a.maxResults, 0, 1<<31-1
	if opts != nil {
		if opts.Since > 0 {
			lower = opts.Since
		}
		if opts.Before > 1 {
			upper = opts.Before - 1
		}
		if opts.Limit > 0 && opts.Limit < limit {
			limit = opts.Limit
		}
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx,
		"SELECT tenant_id,topic,deletedfor,delid,low,hi FROM dellog WHERE tenant_id=? AND topic=? AND delid BETWEEN ? AND ? "+
			"AND (deletedfor=0 OR deletedfor=?) ORDER BY delid LIMIT ?", id, topic, lower, upper, decodeUID(uid), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dmsgs []t.DelMessage
	var current t.DelMessage
	for rows.Next() {
		var row struct {
			TenantID   t.TenantID `db:"tenant_id"`
			Topic      string
			Deletedfor int64
			Delid      int
			Low        int
			Hi         int
		}
		if err = rows.StructScan(&row); err != nil {
			return nil, err
		}
		if row.Delid != current.DelId {
			if current.DelId > 0 {
				dmsgs = append(dmsgs, current)
			}
			current = t.DelMessage{TenantID: row.TenantID, Topic: row.Topic, DelId: row.Delid}
			if row.Deletedfor > 0 {
				current.DeletedFor = store.EncodeUid(row.Deletedfor).String()
			}
		}
		if row.Hi <= row.Low+1 {
			row.Hi = 0
		}
		current.SeqIdRanges = append(current.SeqIdRanges, t.Range{Low: row.Low, Hi: row.Hi})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if current.DelId > 0 {
		dmsgs = append(dmsgs, current)
	}
	return dmsgs, nil
}

func (a *adapter) TenantDeviceUpsert(id t.TenantID, uid t.Uid, dev *t.DeviceDef) error {
	if err := validTenant(id); err != nil || dev == nil || dev.TenantID != id || uid.IsZero() {
		return t.ErrMalformed
	}
	hash := deviceHasher(dev.DeviceId)
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if _, err = tx.Exec("DELETE FROM devices WHERE tenant_id=? AND hash=?", id, hash); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO devices(tenant_id,userid,hash,deviceid,platform,lastseen,lang) VALUES(?,?,?,?,?,?,?)",
		id, decodeUID(uid), hash, dev.DeviceId, dev.Platform, dev.LastSeen, dev.Lang); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *adapter) TenantDeviceGetAll(id t.TenantID, uids ...t.Uid) (map[t.Uid][]t.DeviceDef, int, error) {
	if err := validTenant(id); err != nil {
		return nil, 0, err
	}
	var decoded []any
	for _, uid := range uids {
		if !uid.IsZero() {
			decoded = append(decoded, decodeUID(uid))
		}
	}
	if len(decoded) == 0 {
		return map[t.Uid][]t.DeviceDef{}, 0, nil
	}
	q, args, _ := sqlx.In("SELECT tenant_id,userid,deviceid,platform,lastseen,lang FROM devices WHERE tenant_id=? AND userid IN (?)", id, decoded)
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	rows, err := a.db.QueryxContext(ctx, a.db.Rebind(q), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make(map[t.Uid][]t.DeviceDef)
	count := 0
	for rows.Next() {
		var row struct {
			TenantID t.TenantID `db:"tenant_id"`
			Userid   int64
			Deviceid string
			Platform string
			Lastseen time.Time
			Lang     string
		}
		if err = rows.StructScan(&row); err != nil {
			return nil, 0, err
		}
		uid := store.EncodeUid(row.Userid)
		result[uid] = append(result[uid], t.DeviceDef{
			TenantID: row.TenantID, DeviceId: row.Deviceid, Platform: row.Platform, LastSeen: row.Lastseen, Lang: row.Lang,
		})
		count++
	}
	return result, count, rows.Err()
}

func (a *adapter) TenantDeviceDelete(id t.TenantID, uid t.Uid, deviceID string) error {
	if err := validTenant(id); err != nil || uid.IsZero() {
		return t.ErrMalformed
	}
	q := "DELETE FROM devices WHERE tenant_id=? AND userid=?"
	args := []any{id, decodeUID(uid)}
	if deviceID != "" {
		q += " AND hash=?"
		args = append(args, deviceHasher(deviceID))
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	res, err := a.db.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if count, _ := res.RowsAffected(); count == 0 {
		return t.ErrNotFound
	}
	return nil
}

func (a *adapter) TenantFileStartUpload(id t.TenantID, fd *t.FileDef) error {
	if err := validTenant(id); err != nil || fd == nil || fd.TenantID != id {
		return t.ErrMalformed
	}
	var user any = 0
	if fd.User != "" {
		user = decodeUID(t.ParseUid(fd.User))
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	_, err := a.db.ExecContext(ctx,
		"INSERT INTO fileuploads(tenant_id,id,createdat,updatedat,userid,status,mimetype,size,etag,location) VALUES(?,?,?,?,?,?,?,?,?,?)",
		id, decodeUID(fd.Uid()), fd.CreatedAt, fd.UpdatedAt, user, fd.Status, fd.MimeType, fd.Size, fd.ETag, fd.Location)
	return err
}

func (a *adapter) TenantFileFinishUpload(id t.TenantID, fd *t.FileDef, success bool, size int64) (*t.FileDef, error) {
	if err := validTenant(id); err != nil || fd == nil || fd.TenantID != id {
		return nil, t.ErrMalformed
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	now := t.TimeNow()
	if success {
		_, err = tx.Exec("UPDATE fileuploads SET updatedat=?,status=?,size=?,etag=?,location=? WHERE tenant_id=? AND id=?",
			now, t.UploadCompleted, size, fd.ETag, fd.Location, id, decodeUID(fd.Uid()))
		fd.Status = t.UploadCompleted
		fd.Size = size
	} else {
		_, err = tx.Exec("DELETE FROM fileuploads WHERE tenant_id=? AND id=?", id, decodeUID(fd.Uid()))
		fd.Status = t.UploadFailed
		fd.Size = 0
	}
	if err != nil {
		return nil, err
	}
	fd.UpdatedAt = now
	return fd, tx.Commit()
}

func (a *adapter) TenantFileGet(id t.TenantID, fid string) (*t.FileDef, error) {
	if err := validTenant(id); err != nil {
		return nil, err
	}
	fileID := t.ParseUid(fid)
	if fileID.IsZero() {
		return nil, t.ErrMalformed
	}
	ctx, cancel := a.getContext()
	if cancel != nil {
		defer cancel()
	}
	var fd t.FileDef
	err := a.db.GetContext(ctx, &fd,
		"SELECT tenant_id,id,createdat,updatedat,userid AS user,status,mimetype,size,IFNULL(etag,'') AS etag,location "+
			"FROM fileuploads WHERE tenant_id=? AND id=?", id, decodeUID(fileID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fd.Id = common.EncodeUidString(fd.Id).String()
	fd.User = common.EncodeUidString(fd.User).String()
	return &fd, nil
}

func (a *adapter) TenantFileDeleteUnused(id t.TenantID, olderThan time.Time, limit int) ([]string, error) {
	if err := validTenant(id); err != nil {
		return nil, err
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	query := "SELECT fu.id,fu.location FROM fileuploads AS fu LEFT JOIN filemsglinks AS fml ON fml.tenant_id=fu.tenant_id AND fml.fileid=fu.id WHERE fu.tenant_id=? AND fml.id IS NULL"
	args := []any{id}
	if !olderThan.IsZero() {
		query += " AND fu.updatedat<?"
		args = append(args, olderThan)
	}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	var locations []string
	var ids []any
	for rows.Next() {
		var fileID int64
		var location string
		if err = rows.Scan(&fileID, &location); err != nil {
			rows.Close()
			return nil, err
		}
		if location != "" {
			locations = append(locations, location)
		}
		ids = append(ids, fileID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		q, args, _ := sqlx.In("DELETE FROM fileuploads WHERE tenant_id=? AND id IN (?)", id, ids)
		if _, err = tx.Exec(tx.Rebind(q), args...); err != nil {
			return nil, err
		}
	}
	return locations, tx.Commit()
}

func (a *adapter) TenantFileLinkAttachments(id t.TenantID, topic string, userID, msgID t.Uid, fids []string) error {
	if err := validTenant(id); err != nil || len(fids) == 0 || (topic == "" && msgID.IsZero() && userID.IsZero()) {
		return t.ErrMalformed
	}
	now := t.TimeNow()
	var linkID any
	var linkBy string
	if !msgID.IsZero() {
		linkBy = "msgid"
		linkID = int64(msgID)
	} else if topic != "" {
		linkBy = "topic"
		linkID = topic
		fids = fids[:1]
	} else {
		linkBy = "userid"
		linkID = decodeUID(userID)
		fids = fids[:1]
	}
	var args []any
	for _, fid := range fids {
		fileID := t.ParseUid(fid)
		if fileID.IsZero() {
			return t.ErrMalformed
		}
		args = append(args, id, now, decodeUID(fileID), linkID)
	}
	ctx, cancel := a.getContextForTx()
	if cancel != nil {
		defer cancel()
	}
	tx, err := a.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if msgID.IsZero() {
		if _, err = tx.Exec("DELETE FROM filemsglinks WHERE tenant_id=? AND "+linkBy+"=?", id, linkID); err != nil {
			return err
		}
	}
	sqlq := "INSERT INTO filemsglinks(tenant_id,createdat,fileid," + linkBy + ") VALUES (?,?,?,?)"
	_, err = tx.Exec(sqlq+strings.Repeat(",(?,?,?,?)", len(fids)-1), args...)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (a *adapter) TenantPCacheGet(id t.TenantID, key string) (string, error) {
	if err := validTenant(id); err != nil {
		return "", err
	}
	scoped, err := tenantCacheKey(id, key)
	if err != nil {
		return "", err
	}
	return a.PCacheGet(scoped)
}

func (a *adapter) TenantPCacheUpsert(id t.TenantID, key, value string, fail bool) error {
	if err := validTenant(id); err != nil {
		return err
	}
	scoped, err := tenantCacheKey(id, key)
	if err != nil {
		return err
	}
	return a.PCacheUpsert(scoped, value, fail)
}

func (a *adapter) TenantPCacheDelete(id t.TenantID, key string) error {
	if err := validTenant(id); err != nil {
		return err
	}
	scoped, err := tenantCacheKey(id, key)
	if err != nil {
		return err
	}
	return a.PCacheDelete(scoped)
}

func (a *adapter) TenantPCacheExpire(id t.TenantID, prefix string, before time.Time) error {
	if err := validTenant(id); err != nil {
		return err
	}
	scoped, err := tenantCacheKey(id, prefix)
	if err != nil {
		return err
	}
	return a.PCacheExpire(scoped, before)
}
