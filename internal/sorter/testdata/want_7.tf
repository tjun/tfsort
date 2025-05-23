resource "a" "concat_list1" {
  foods = concat(
    [ #fruits
      "apple",
      "banana",
      "cherry",
    ],
    [ #vegetables
      "broccoli",
      "carrot",
      "tomato",
    ]
  )
}

resource "a" "concat_list2" {
  foods = concat(
    #fruits
    [
      "apple",
      "banana",
      "cherry",
    ],
    #vegetables
    [
      "broccoli",
      "carrot",
      "tomato",
    ]
  )
}

resource "a" "concat_list3" {
  foods = concat(
    [
      "apple",
      "banana",
      #fruits
      "cherry",
    ],
    [
      "broccoli",
      "carrot",
      #vegetables
      "tomato",
    ]
  )
}
