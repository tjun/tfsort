resource "list" "fruits" {

  foods = [
    "apple",
    "banana",
    "orange",
  ]
}

resource "list_blocks" "vegetables" {
  # keep comments on the same line as the value
  foods = [
    "carrot", # carrot
    "potato", # potato
    "tomato", # tomato
  ]
}
