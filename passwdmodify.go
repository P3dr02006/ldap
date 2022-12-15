package ldap

import (
	"context"
	"fmt"

	ber "github.com/go-asn1-ber/asn1-ber"
)

const (
	passwordModifyOID = "1.3.6.1.4.1.4203.1.11.1"
)

// PasswordModifyRequest implements the Password Modify Extended Operation as defined in https://www.ietf.org/rfc/rfc3062.txt
type PasswordModifyRequest struct {
	// UserIdentity is an optional string representation of the user associated with the request.
	// This string may or may not be an LDAPDN [RFC2253].
	// If no UserIdentity field is present, the request acts up upon the password of the user currently associated with the LDAP session
	UserIdentity string
	// OldPassword, if present, contains the user's current password
	OldPassword string
	// NewPassword, if present, contains the desired password for this user
	NewPassword string
}

// PasswordModifyResult holds the server response to a PasswordModifyRequest
type PasswordModifyResult struct {
	// GeneratedPassword holds a password generated by the server, if present
	GeneratedPassword string
	// Referral are the returned referral
	Referral string
}

func (req *PasswordModifyRequest) appendTo(envelope *ber.Packet) error {
	pkt := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ApplicationExtendedRequest, nil, "Password Modify Extended Operation")
	pkt.AppendChild(ber.NewString(ber.ClassContext, ber.TypePrimitive, 0, passwordModifyOID, "Extended Request Name: Password Modify OID"))

	extendedRequestValue := ber.Encode(ber.ClassContext, ber.TypePrimitive, 1, nil, "Extended Request Value: Password Modify Request")
	passwordModifyRequestValue := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "Password Modify Request")
	if req.UserIdentity != "" {
		passwordModifyRequestValue.AppendChild(ber.NewString(ber.ClassContext, ber.TypePrimitive, 0, req.UserIdentity, "User Identity"))
	}
	if req.OldPassword != "" {
		passwordModifyRequestValue.AppendChild(ber.NewString(ber.ClassContext, ber.TypePrimitive, 1, req.OldPassword, "Old Password"))
	}
	if req.NewPassword != "" {
		passwordModifyRequestValue.AppendChild(ber.NewString(ber.ClassContext, ber.TypePrimitive, 2, req.NewPassword, "New Password"))
	}
	extendedRequestValue.AppendChild(passwordModifyRequestValue)

	pkt.AppendChild(extendedRequestValue)

	envelope.AppendChild(pkt)

	return nil
}

// NewPasswordModifyRequest creates a new PasswordModifyRequest
//
// According to the RFC 3602 (https://tools.ietf.org/html/rfc3062):
// userIdentity is a string representing the user associated with the request.
// This string may or may not be an LDAPDN (RFC 2253).
// If userIdentity is empty then the operation will act on the user associated
// with the session.
//
// oldPassword is the current user's password, it can be empty or it can be
// needed depending on the session user access rights (usually an administrator
// can change a user's password without knowing the current one) and the
// password policy (see pwdSafeModify password policy's attribute)
//
// newPassword is the desired user's password. If empty the server can return
// an error or generate a new password that will be available in the
// PasswordModifyResult.GeneratedPassword
//
func NewPasswordModifyRequest(userIdentity string, oldPassword string, newPassword string) *PasswordModifyRequest {
	return &PasswordModifyRequest{
		UserIdentity: userIdentity,
		OldPassword:  oldPassword,
		NewPassword:  newPassword,
	}
}

// PasswordModify performs the modification request
func (l *Conn) PasswordModify(passwordModifyRequest *PasswordModifyRequest) (*PasswordModifyResult, error) {
	return l.PasswordModifyContext(l.ctx, passwordModifyRequest)
}

// PasswordModify performs the modification request
func (l *Conn) PasswordModifyContext(ctx context.Context, passwordModifyRequest *PasswordModifyRequest) (*PasswordModifyResult, error) {
	msgCtx, err := l.doRequest(ctx, passwordModifyRequest)
	if err != nil {
		return nil, err
	}
	defer l.finishMessage(msgCtx)

	packet, err := l.readPacket(msgCtx)
	if err != nil {
		return nil, err
	}

	result := &PasswordModifyResult{}

	if packet.Children[1].Tag == ApplicationExtendedResponse {
		if err = GetLDAPError(packet); err != nil {
			if referral, referralErr := getReferral(err, packet); referralErr != nil {
				return result, referralErr
			} else {
				result.Referral = referral
			}

			return result, err
		}
	} else {
		return nil, NewError(ErrorUnexpectedResponse, fmt.Errorf("unexpected Response: %d", packet.Children[1].Tag))
	}

	extendedResponse := packet.Children[1]
	for _, child := range extendedResponse.Children {
		if child.Tag == ber.TagEmbeddedPDV {
			passwordModifyResponseValue := ber.DecodePacket(child.Data.Bytes())
			if len(passwordModifyResponseValue.Children) == 1 {
				if passwordModifyResponseValue.Children[0].Tag == ber.TagEOC {
					result.GeneratedPassword = ber.DecodeString(passwordModifyResponseValue.Children[0].Data.Bytes())
				}
			}
		}
	}

	return result, nil
}
