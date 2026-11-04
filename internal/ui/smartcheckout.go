package viffui

import (
	vapp "viff/internal/app"
	"viff/internal/domain"
)

// confirmStashCheckout asks before stashing local changes to finish a
// checkout Git blocked. Returning true tells the workspace the dialog owns
// the error; nothing is stashed unless the user picks "Stash and Checkout".
func (w *Window) confirmStashCheckout(b vapp.CheckoutBlocked) bool {
	name := b.Ref.Short
	if b.Rev != "" {
		name = domain.ShortHash(b.Rev)
	}
	w.confirm("Stash Changes and Checkout",
		"Your local changes would be overwritten by checking out "+name+".\n\n"+
			"Viff can stash them, including untracked files, check out, and restore them afterwards. "+
			"If restoring them conflicts, they stay in the stash.",
		"Stash and Checkout", func() { w.ws.SmartCheckout(b) })
	return true
}
