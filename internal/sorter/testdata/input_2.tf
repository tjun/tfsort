# comment blocks
resource "list_blocks" "a" {
  a = "b"

  foods = [
    # fruits
    "banana",
    "apple",
    "orange",

    # vegetables
    "potato",
    "tomato",
    "carrot",

    # meats
    "chicken",
    "beef",
    "pork",
  ]
}
