package remotedialer

import (
	"context"
	"math/rand"
)

// ServeConn registers clientKey's session from an already-open conn instead
// of upgrading an *http.Request, which is what a Durable Object needs: the
// socket comes from the Workers runtime's hibernatable WebSocket API, not
// from an http.Hijacker. It runs the session (as ServeHTTP does after its
// own upgrade) and removes it on exit. Added by scripts/mirror.
func (s *Server) ServeConn(clientKey string, conn wsConn) error {
	sessionKey := rand.Int63()
	session := newSession(sessionKey, clientKey, conn)
	session.auth = s.ClientConnectAuthorizer

	s.sessions.Lock()
	s.sessions.clients[clientKey] = append(s.sessions.clients[clientKey], session)
	for l := range s.sessions.listeners {
		l.sessionAdded(clientKey, session.sessionKey)
	}
	s.sessions.Unlock()

	defer s.sessions.remove(session)

	_, err := session.Serve(context.Background())
	return err
}
