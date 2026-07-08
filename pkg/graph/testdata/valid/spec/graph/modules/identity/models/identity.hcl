entity "User" {
  key = ["id"]

  property "id" {
    type = "uuid"
  }
}

entity "Team" {
  key = ["id"]

  property "id" {
    type = "uuid"
  }
}

enum "TeamRole" {
  values = ["owner", "member"]
}
