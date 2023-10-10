package scraper

import (
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func toPGTime(t metav1.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func ptrToPGTime(t *metav1.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func parsePGUUID(src types.UID) (pgtype.UUID, error) {
	uid, err := parseUUID(string(src))
	return pgtype.UUID{Bytes: uid, Valid: err == nil}, err
}

func controller(ref []metav1.OwnerReference) (uid pgtype.UUID, kind, name string) {
	for _, r := range ref {
		if r.Controller != nil && *r.Controller {
			uid, _ = parsePGUUID(r.UID)
			return uid, r.Kind, r.Name
		}
	}
	return pgtype.UUID{}, "", ""
}

// parseUUID converts a string UUID in standard form to a byte array.
func parseUUID(src string) (dst [16]byte, err error) {
	switch len(src) {
	case 36:
		src = src[0:8] + src[9:13] + src[14:18] + src[19:23] + src[24:]
	case 32:
		// dashes already stripped, assume valid
	default:
		// assume invalid.
		return dst, fmt.Errorf("cannot parse UUID %v", src)
	}

	buf, err := hex.DecodeString(src)
	if err != nil {
		return dst, fmt.Errorf("cannot parse UUID %v: %w", src, err)
	}

	copy(dst[:], buf)
	return dst, nil
}
