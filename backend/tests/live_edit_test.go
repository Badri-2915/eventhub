package tests

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/eventhub/backend/internal/events"
	"github.com/eventhub/backend/internal/inventory"
)

// inputFrom turns a saved event back into the wizard's PUT body.
func inputFrom(ev events.EventDTO) events.EventInput {
	in := events.EventInput{
		Title: ev.Title, Description: ev.Description, Category: ev.Category, MapURL: ev.MapURL,
		VenueName: ev.VenueName, City: ev.City, Address: ev.Address, StartsAt: ev.StartsAt, EndsAt: ev.EndsAt,
		SeatingMode: ev.SeatingMode, Layout: ev.Layout, MaxPerBooking: &ev.MaxPerBooking, CancelCutoffHours: &ev.CancelCutoffHours,
	}
	for _, c := range ev.Categories {
		in.Categories = append(in.Categories, events.CategoryInput{
			ID: &c.ID, Name: c.Name, Color: c.Color, ListPricePaise: c.ListPricePaise,
			DiscountPercent: c.DiscountPercent, Capacity: c.Capacity,
		})
	}
	return in
}

func TestLiveEditPriceAppliesToNewBookingsOnly(t *testing.T) {
	ctx := context.Background()
	ev, cat := publishGA(t, 20, 10)
	early := newUser(t)
	h1, err := invSvc.Hold(ctx, early, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}

	in := inputFrom(ev)
	in.Title = "Renamed while live"
	in.Categories[0].ListPricePaise = 50000 // ₹500, 20% off → ₹400
	in.Categories[0].DiscountPercent = 20
	updated, err := evSvc.Update(ctx, ev.HostID, ev.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Renamed while live" || updated.Categories[0].PricePaise != 40000 {
		t.Fatalf("want new title and ₹400 price, got %q %d", updated.Title, updated.Categories[0].PricePaise)
	}

	b1, err := invSvc.Confirm(ctx, early, h1.BookingID, uuid.NewString(), false)
	if err != nil {
		t.Fatal(err)
	}
	if b1.TotalPaise != 2*49900 {
		t.Fatalf("a hold placed before the edit keeps its price: want %d, got %d", 2*49900, b1.TotalPaise)
	}
	h2, err := invSvc.Hold(ctx, newUser(t), inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if h2.TotalPaise != 40000 {
		t.Fatalf("new holds use the discounted price: want 40000, got %d", h2.TotalPaise)
	}
}

func TestLiveEditGuards(t *testing.T) {
	ctx := context.Background()
	ev, cat := publishGA(t, 10, 10)
	if _, err := invSvc.Hold(ctx, newUser(t), inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: cat, Qty: 4}}}); err != nil {
		t.Fatal(err)
	}

	in := inputFrom(ev)
	three := int32(3)
	in.Categories[0].Capacity = &three
	if _, err := evSvc.Update(ctx, ev.HostID, ev.ID, in); code(err) != "LIVE_EDIT_INVALID" {
		t.Fatalf("capacity below tickets on hold must fail, got %v", err)
	}
	more := int32(50)
	in.Categories[0].Capacity = &more
	if _, err := evSvc.Update(ctx, ev.HostID, ev.ID, in); err != nil {
		t.Fatalf("raising capacity must work: %v", err)
	}

	in = inputFrom(ev)
	in.Categories = append(in.Categories, events.CategoryInput{Name: "VIP", ListPricePaise: 99900, Capacity: &more})
	if _, err := evSvc.Update(ctx, ev.HostID, ev.ID, in); code(err) != "LIVE_EDIT_INVALID" {
		t.Fatalf("adding a ticket type to a live event must fail, got %v", err)
	}
	in = inputFrom(ev)
	in.SeatingMode = "NUMBERED"
	if _, err := evSvc.Update(ctx, ev.HostID, ev.ID, in); code(err) != "LIVE_EDIT_INVALID" {
		t.Fatalf("changing seating of a live event must fail, got %v", err)
	}

	if _, err := evSvc.Update(ctx, newHost(t), ev.ID, inputFrom(ev)); code(err) != "NOT_FOUND" {
		t.Fatalf("another host must not edit it, got %v", err)
	}
	if _, err := invSvc.CancelEvent(ctx, ev.HostID, ev.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := evSvc.Update(ctx, ev.HostID, ev.ID, inputFrom(ev)); code(err) != "EVENT_NOT_EDITABLE" {
		t.Fatalf("cancelled events are read-only, got %v", err)
	}
}

func TestFreeTicketsBookAtZero(t *testing.T) {
	ctx := context.Background()
	host := newHost(t)
	capacity := int32(5)
	in := baseInput("GA")
	in.Categories = []events.CategoryInput{{Name: "Entry", ListPricePaise: 0, DiscountPercent: 30, Capacity: &capacity}}
	ev, err := evSvc.CreateDraft(ctx, host, in)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Categories[0].PricePaise != 0 || ev.Categories[0].DiscountPercent != 0 {
		t.Fatalf("free ticket must cost 0 with no discount, got %+v", ev.Categories[0])
	}
	if ev, err = evSvc.Publish(ctx, host, ev.ID); err != nil {
		t.Fatal(err)
	}
	u := newUser(t)
	h, err := invSvc.Hold(ctx, u, inventory.HoldRequest{EventID: ev.ID, Items: []inventory.GAItem{{CategoryID: ev.Categories[0].ID, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := invSvc.Confirm(ctx, u, h.BookingID, uuid.NewString(), false)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != "CONFIRMED" || b.TotalPaise != 0 || b.TicketCode == nil {
		t.Fatalf("free booking should confirm at ₹0 with a ticket, got %s %d", b.Status, b.TotalPaise)
	}
}
