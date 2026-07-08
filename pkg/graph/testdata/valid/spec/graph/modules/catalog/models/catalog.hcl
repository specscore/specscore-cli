entity "Asset" {
  key = ["id"]

  property "id" {
    type = "uuid"
  }

  property "category" {
    type = "string"
    enum = "AssetCategory"
  }
}

enum "AssetCategory" {
  values = ["tool", "room"]
}
