package server

import (
	"context"
	"io/ioutil"
	"net/http"

	"github.com/keys-pub/keys"
	"github.com/keys-pub/keys-ext/http/api"
	wsapi "github.com/keys-pub/keys-ext/ws/api"
	"github.com/keys-pub/keys/dstore"
	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/vmihailenco/msgpack/v4"
)

func (s *Server) postMessage(c echo.Context) error {
	s.logger.Infof("Server %s %s", c.Request().Method, c.Request().URL.String())

	if c.Request().Body == nil {
		return ErrBadRequest(c, errors.Errorf("missing body"))
	}

	b, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return s.internalError(c, err)
	}

	if len(b) > 16*1024 {
		// TODO: Check length before reading data
		return ErrBadRequest(c, errors.Errorf("message too large (greater than 16KiB)"))
	}

	channel, _, err := s.authChannel(c, "cid", b)
	if err != nil {
		return ErrForbidden(c, err)
	}

	ctx := c.Request().Context()
	path := dstore.Path("channels", channel.KID)

	events, idx, err := s.fi.EventsAdd(ctx, path, [][]byte{b})
	if err != nil {
		return s.internalError(c, err)
	}
	if len(events) == 0 {
		return ErrBadRequest(c, errors.Errorf("no events added"))
	}

	// Notify channel
	if err := s.notifyChannel(ctx, channel.KID, idx); err != nil {
		return s.internalError(c, err)
	}

	var out struct{}
	return JSON(c, http.StatusOK, out)
}

func (s *Server) notifyChannel(ctx context.Context, channel keys.ID, idx int64) error {
	users, err := s.channelUserIDs(ctx, channel)
	if err != nil {
		return err
	}
	pub := &wsapi.PubEvent{
		Channel: channel,
		Users:   users,
		Index:   idx,
	}
	pb, err := msgpack.Marshal(pub)
	if err != nil {
		return err
	}
	if err := s.rds.Publish(ctx, wsapi.EventPubSub, pb); err != nil {
		return err
	}
	return nil
}

func (s *Server) listMessages(c echo.Context) error {
	s.logger.Infof("Server %s %s", c.Request().Method, c.Request().URL.String())

	channel, _, err := s.authChannel(c, "cid", nil)
	if err != nil {
		return ErrForbidden(c, err)
	}

	path := dstore.Path("channels", channel.KID)
	resp, err := s.events(c, path, 1000)
	if err != nil {
		return s.internalError(c, err)
	}

	out := &api.MessagesResponse{
		Messages: resp.Events,
		Index:    resp.Index,
	}

	return JSON(c, http.StatusOK, out)
}
