entity "Widget" {
  key = ["id"]

  property "id" {
    type = "uuid"
  }

  property "space" {
    entity = "core.Space"
  }
}
