//go:build e2e

// gpg.go generates the OpenPGP public keys the GPG key scenarios add to an
// account.
//
// A key is generated rather than embedded because GitLab refuses a fingerprint
// any account on the instance already holds: the suite this replaces carried
// two fixed keys, could add each of them exactly once at a time, and so ran its
// GPG scenarios on the disposable Docker instance only and on one surface. A
// fresh key per call has no such limit.
//
// The packets are written here, out of the standard library, rather than taken
// from golang.org/x/crypto/openpgp, which is the obvious way to do this and is
// closed to us: that package is the subject of GO-2026-5932 ("unmaintained,
// unsafe by design"), the repository's vulnerability gate keeps an empty
// allowlist on purpose, and `make analyze` runs govulncheck over this package
// with the e2e tag. Importing it here fails that gate with twenty-six call
// traces into this one file. What is written is the minimum GnuPG accepts on
// import, which is what GitLab runs the key through: a version 4 RSA public
// key packet, a user id, and the positive certification binding the two.
//
// RFC 4880 is the reference for every constant and every layout below, and the
// fixture's own test hands a generated key to the real gpg, which is the only
// judge of whether they were read correctly.

package fixture

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // G505: SHA-1 is not a choice here: RFC 4880 defines a version 4 key fingerprint as a SHA-1 digest, and that fingerprint is an identifier GitLab looks the key up by, never a security claim of ours.
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// The sizes and identifiers a generated key carries.
const (
	// gpgKeyBits is the RSA size of a generated key. GitLab accepts anything
	// gpg imports, and 2048 keeps generation well under a second per key.
	gpgKeyBits = 2048
	// gpgUserEmailDomain is the domain the key's user id names, one that
	// resolves nowhere, like the fixture users' own.
	gpgUserEmailDomain = "@e2e-test.invalid"
)

// The OpenPGP packet tags, algorithm identifiers and signature constants this
// file writes, each as RFC 4880 numbers them.
const (
	packetTagSignature = 2
	packetTagPublicKey = 6
	packetTagUserID    = 13

	publicKeyVersion = 4
	signatureVersion = 4

	// algoRSA is public-key algorithm 1, RSA (encrypt or sign), which is what
	// GnuPG and GPGME accept everywhere without a compile-time option.
	algoRSA = 1
	// hashSHA256 is hash algorithm 8.
	hashSHA256 = 8

	// sigTypePositiveCert is signature type 0x13, the positive certification
	// of a user id and public-key packet: the self-signature that makes the
	// user id usable, and without which gpg imports nothing.
	sigTypePositiveCert = 0x13

	// The subpacket types written below: the signature's creation time, the
	// key's flags, and the issuer's key id.
	subpacketCreationTime = 2
	subpacketIssuer       = 16
	subpacketKeyFlags     = 27
	// keyFlagsCertifyAndSign is the flags octet: certify (0x01) and sign
	// (0x02), which is what a primary key with no subkey does.
	keyFlagsCertifyAndSign = 0x03
)

// The prefixes RFC 4880 section 5.2.4 hashes a certification over.
const (
	hashPrefixPublicKey = 0x99
	hashPrefixUserID    = 0xB4
	trailerVersion      = 0x04
	trailerConstant     = 0xFF
)

// GPGPublicKey generates a fresh OpenPGP key pair and returns the public half
// as an ASCII-armored block, the form the GPG key actions take. The private
// half is discarded once it has signed the user id: nothing in a test ever
// signs with it. The user id names the given owner, so a key added to a
// fixture user reads as that user's.
func GPGPublicKey(owner string) (string, error) {
	private, err := rsa.GenerateKey(rand.Reader, gpgKeyBits)
	if err != nil {
		return "", fmt.Errorf("generating an RSA key for %s: %w", owner, err)
	}

	created := int(time.Now().Unix())
	publicKey := publicKeyPacketBody(&private.PublicKey, created)
	userID := fmt.Sprintf("E2E %s (e2e fixture) <%s%s>", owner, owner, gpgUserEmailDomain)

	certification, err := certifyUserID(private, publicKey, userID, created)
	if err != nil {
		return "", fmt.Errorf("certifying the key for %s: %w", owner, err)
	}

	packets := appendPacket(nil, packetTagPublicKey, publicKey)
	packets = appendPacket(packets, packetTagUserID, []byte(userID))
	packets = appendPacket(packets, packetTagSignature, certification)
	return armor(packets), nil
}

// publicKeyPacketBody writes the body of a version 4 RSA public-key packet:
// the version, the creation time, the algorithm and the two values of the
// public key.
func publicKeyPacketBody(public *rsa.PublicKey, created int) []byte {
	body := []byte{publicKeyVersion}
	body = appendBigEndian(body, 4, created)
	body = append(body, algoRSA)
	body = append(body, mpi(public.N)...)
	return append(body, mpi(big.NewInt(int64(public.E)))...)
}

// certifyUserID writes the self-signature that binds a user id to the key: the
// packet body of a version 4 positive certification, signed with the private
// half.
//
// The hash is taken over the four blocks section 5.2.4 names, in order: the
// public key packet under its 0x99 prefix, the user id under its 0xB4 prefix,
// the signature's own data down to the end of its hashed subpackets, and the
// trailer counting that last block. Anything left out or counted twice gives a
// signature gpg rejects, which is what the fixture's own test checks against
// the real gpg.
func certifyUserID(private *rsa.PrivateKey, publicKey []byte, userID string, created int) ([]byte, error) {
	hashed := subpackets(
		subpacket(subpacketCreationTime, appendBigEndian(nil, 4, created)),
		subpacket(subpacketKeyFlags, []byte{keyFlagsCertifyAndSign}),
	)

	// The signature's data from the version octet to the end of the hashed
	// subpackets: hashed as part of the digest and written into the packet.
	signed := append([]byte{signatureVersion, sigTypePositiveCert, algoRSA, hashSHA256}, hashed...)

	digest := sha256.New()
	digest.Write(hashablePublicKey(publicKey))
	digest.Write(appendBigEndian([]byte{hashPrefixUserID}, 4, len(userID)))
	digest.Write([]byte(userID))
	digest.Write(signed)
	digest.Write(appendBigEndian([]byte{trailerVersion, trailerConstant}, 4, len(signed)))
	sum := digest.Sum(nil)

	signature, err := rsa.SignPKCS1v15(rand.Reader, private, crypto.SHA256, sum)
	if err != nil {
		return nil, fmt.Errorf("signing the user id: %w", err)
	}

	body := append([]byte(nil), signed...)
	body = append(body, subpackets(subpacket(subpacketIssuer, keyID(publicKey)))...)
	// The first two octets of the digest, which let a reader discard a wrong
	// key before doing any public-key arithmetic.
	body = append(body, sum[:2]...)
	return append(body, mpi(new(big.Int).SetBytes(signature))...), nil
}

// hashablePublicKey returns the public key packet in the form a signature
// hashes it: the 0x99 prefix, the body's two-octet length, and the body.
func hashablePublicKey(publicKey []byte) []byte {
	return append(appendBigEndian([]byte{hashPrefixPublicKey}, 2, len(publicKey)), publicKey...)
}

// fingerprint returns the version 4 fingerprint of a public key: the SHA-1 of
// the same block a signature hashes.
func fingerprint(publicKey []byte) []byte {
	digest := sha1.New() //nolint:gosec // G401: see the sha1 import.
	digest.Write(hashablePublicKey(publicKey))
	return digest.Sum(nil)
}

// keyID returns the key's id: the last eight octets of its fingerprint.
func keyID(publicKey []byte) []byte {
	full := fingerprint(publicKey)
	return full[len(full)-8:]
}

// mpi encodes a multiprecision integer: the number of significant bits, then
// the value's own big-endian octets.
func mpi(value *big.Int) []byte {
	return append(appendBigEndian(nil, 2, value.BitLen()), value.Bytes()...)
}

// subpacket encodes one signature subpacket: its length, its type and its
// data. Every subpacket written here is far below the 192-octet boundary where
// the length stops being one octet.
func subpacket(kind byte, data []byte) []byte {
	return append(appendBigEndian(nil, 1, len(data)+1), append([]byte{kind}, data...)...)
}

// subpackets concatenates subpackets behind the two-octet length the signature
// packet introduces each of its two sets with.
func subpackets(parts ...[]byte) []byte {
	var joined []byte
	for _, part := range parts {
		joined = append(joined, part...)
	}
	return append(appendBigEndian(nil, 2, len(joined)), joined...)
}

// appendPacket frames a packet body with a new-format header: the tag, then the
// body length in the shortest of the three forms that holds it.
func appendPacket(out []byte, tag byte, body []byte) []byte {
	out = append(out, 0xC0|tag)
	switch length := len(body); {
	case length < 192:
		out = appendBigEndian(out, 1, length)
	case length < 8384:
		out = appendBigEndian(out, 1, (length-192)>>8+192)
		out = appendBigEndian(out, 1, (length-192)&0xFF)
	default:
		out = append(out, 0xFF)
		out = appendBigEndian(out, 4, length)
	}
	return append(out, body...)
}

// appendBigEndian appends the low width octets of value, most significant
// first.
//
// Every length, count and checksum this file writes goes through here, so the
// one narrowing conversion in the package is the masked one below rather than
// a dozen at the call sites. Each caller's value is a length or a count of its
// own packet, orders below the width it asks for.
func appendBigEndian(out []byte, width, value int) []byte {
	for shift := (width - 1) * 8; shift >= 0; shift -= 8 {
		out = append(out, byte((value>>shift)&0xFF))
	}
	return out
}

// armorLineWidth is how many base64 characters an armored line carries.
const armorLineWidth = 64

// armor wraps packets in the ASCII armor of a public key block, with the CRC24
// checksum line RFC 4880 section 6.1 defines.
func armor(packets []byte) string {
	encoded := base64.StdEncoding.EncodeToString(packets)

	var block strings.Builder
	block.WriteString("-----BEGIN PGP PUBLIC KEY BLOCK-----\n\n")
	for start := 0; start < len(encoded); start += armorLineWidth {
		block.WriteString(encoded[start:min(start+armorLineWidth, len(encoded))])
		block.WriteString("\n")
	}
	block.WriteString("=")
	block.WriteString(base64.StdEncoding.EncodeToString(crc24(packets)))
	block.WriteString("\n-----END PGP PUBLIC KEY BLOCK-----\n")
	return block.String()
}

// The CRC24 of the armor checksum line, with the initial value and polynomial
// RFC 4880 states.
const (
	crc24Init = 0xB704CE
	crc24Poly = 0x1864CFB
)

// crc24 computes the armor checksum as its three big-endian octets.
func crc24(data []byte) []byte {
	crc := crc24Init
	for _, octet := range data {
		crc ^= int(octet) << 16
		for range 8 {
			crc <<= 1
			if crc&0x1000000 != 0 {
				crc ^= crc24Poly
			}
		}
	}
	return appendBigEndian(nil, 3, crc&0xFFFFFF)
}
