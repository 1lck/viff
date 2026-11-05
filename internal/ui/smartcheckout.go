package viffui

import (
	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
)

// confirmStashCheckout asks before stashing local changes to finish a
// checkout Git blocked. Returning true tells the workspace the dialog owns
// the error; nothing is stashed unless the user picks "Stash and Checkout".
func (w *Window) confirmStashCheckout(b vapp.CheckoutBlocked) bool {
	name := b.Ref.Short
	if b.Rev != "" {
		name = domain.ShortHash(b.Rev)
	}
	w.confirm(w.t(i18n.WorkspaceSmartCheckoutTitle),
		w.t(i18n.WorkspaceSmartCheckoutBody, name),
		w.t(i18n.WorkspaceSmartCheckoutButton), func() { w.ws.SmartCheckout(b) })
	return true
}
