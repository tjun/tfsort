# comment blocks
resource "list_blocks" "a" {
  a = "b"

  foods = [
    "apple",
    # fruits
    "banana",
    "beef",
    "carrot",

    # meats
    "chicken",
    "orange",
    "pork",

    # vegetables
    "potato",
    "tomato",
  ]
}
