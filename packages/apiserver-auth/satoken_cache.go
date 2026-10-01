package auth

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const saObjectCacheTTL = 10 * time.Second
const saObjectCacheSweepSize = 1024

type objectFacts struct {
	uid       types.UID
	deleted   *metav1.Time
	expiresAt time.Time
}

type CachedObjects struct {
	Objects ServiceAccountObjects
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	facts   map[string]objectFacts
}

func NewCachedObjects(objects ServiceAccountObjects, ttl time.Duration) *CachedObjects {
	return &CachedObjects{Objects: objects, ttl: ttl, now: time.Now, facts: map[string]objectFacts{}}
}

func NewServiceAccountObjects(objects ServiceAccountObjects) *CachedObjects {
	return NewCachedObjects(objects, saObjectCacheTTL)
}

func (c *CachedObjects) lookup(key string, fetch func() (metav1.ObjectMeta, error)) (metav1.ObjectMeta, error) {
	now := c.now()
	c.mu.Lock()
	f, ok := c.facts[key]
	c.mu.Unlock()
	if ok && now.Before(f.expiresAt) {
		return metav1.ObjectMeta{UID: f.uid, DeletionTimestamp: f.deleted}, nil
	}
	meta, err := fetch()
	if err != nil {
		return metav1.ObjectMeta{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.facts) >= saObjectCacheSweepSize {
		for k, v := range c.facts {
			if !now.Before(v.expiresAt) {
				delete(c.facts, k)
			}
		}
	}
	c.facts[key] = objectFacts{uid: meta.UID, deleted: meta.DeletionTimestamp, expiresAt: now.Add(c.ttl)}
	return metav1.ObjectMeta{UID: meta.UID, DeletionTimestamp: meta.DeletionTimestamp}, nil
}

func (c *CachedObjects) ServiceAccount(ctx context.Context, namespace, name string) (*corev1.ServiceAccount, error) {
	meta, err := c.lookup("serviceaccounts/"+namespace+"/"+name, func() (metav1.ObjectMeta, error) {
		o, err := c.Objects.ServiceAccount(ctx, namespace, name)
		if err != nil {
			return metav1.ObjectMeta{}, err
		}
		return o.ObjectMeta, nil
	})
	if err != nil {
		return nil, err
	}
	return &corev1.ServiceAccount{ObjectMeta: meta}, nil
}

func (c *CachedObjects) Pod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	meta, err := c.lookup("pods/"+namespace+"/"+name, func() (metav1.ObjectMeta, error) {
		o, err := c.Objects.Pod(ctx, namespace, name)
		if err != nil {
			return metav1.ObjectMeta{}, err
		}
		return o.ObjectMeta, nil
	})
	if err != nil {
		return nil, err
	}
	return &corev1.Pod{ObjectMeta: meta}, nil
}

func (c *CachedObjects) Secret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	meta, err := c.lookup("secrets/"+namespace+"/"+name, func() (metav1.ObjectMeta, error) {
		o, err := c.Objects.Secret(ctx, namespace, name)
		if err != nil {
			return metav1.ObjectMeta{}, err
		}
		return o.ObjectMeta, nil
	})
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{ObjectMeta: meta}, nil
}

func (c *CachedObjects) LiveSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return c.Objects.Secret(ctx, namespace, name)
}

func (c *CachedObjects) Node(ctx context.Context, name string) (*corev1.Node, error) {
	meta, err := c.lookup("nodes/"+name, func() (metav1.ObjectMeta, error) {
		o, err := c.Objects.Node(ctx, name)
		if err != nil {
			return metav1.ObjectMeta{}, err
		}
		return o.ObjectMeta, nil
	})
	if err != nil {
		return nil, err
	}
	return &corev1.Node{ObjectMeta: meta}, nil
}
