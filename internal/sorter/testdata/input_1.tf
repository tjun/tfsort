resource "list" "fruits" {

  foods = [
    "banana",
    "apple",
    "orange",
  ]
}

resource "list_blocks" "vegetables" {
  # keep comments on the same line as the value
  foods = [
    "potato", # potato
    "tomato", # tomato
    "carrot", # carrot
  ]
}
