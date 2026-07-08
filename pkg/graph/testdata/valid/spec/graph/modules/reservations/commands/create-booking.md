---
kind: command
id: create-booking
name: CreateBooking
status: draft
subject: reservations.booking
actors:
  - identity.user
inputs:
  - name: resource
    ref: catalog.asset
  - name: time-window
    model: modelspec:///scheduling.TimeWindow
possibleEvents:
  - reservations.booking-created
summary: Create a booking.
---

# Command: CreateBooking

## Description

Create a booking for an asset.

## Open Questions

None at this time.
