entity "Booking" {
  key = ["id"]

  property "id" {
    type = "uuid"
  }

  property "team" {
    entity = "identity.Team"
  }

  property "window" {
    component = "scheduling.TimeWindow"
  }

  property "role" {
    type = "string"
    enum = "identity.TeamRole"
  }
}
