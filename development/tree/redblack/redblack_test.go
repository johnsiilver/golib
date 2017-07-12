package redblack

import (
  "reflect"
  "testing"
  //"github.com/kr/pretty"
)

func TestLeftRotation(t *testing.T) {
  tr := &tree{
    root: &node{
      left: &node{  // We are rotating this node, t.root.left
        key: Int(10),
        left: &node{
          key: Int(5),
          left: &node{
            key: Int(3),
          },
          right: &node{
            key: Int(7),
          },
        },
        right: &node{
          key: Int(15),
          left: &node{
            key: Int(13),
          },
          right: &node{
            key: Int(17),
          },
        },
      },
    },
  }

  want := &tree{
    root: &node{
      left: &node{
        key: Int(7),
        left: &node{
          key: Int(10),
          left: &node{
            key: Int(5),
            left: &node{
              key: Int(3),
            },
          },
        },
        right: &node{
            key: Int(15),
            left: &node{
              key: Int(13),
            },
            right: &node{
              key: Int(17),
            },
        },
      },
    },
  }

  tr.leftRotate(tr.root.left)

  /*
  if diff := pretty.Compare(t, want); diff != "" {
    t.Errorf("-want/+got:\n%s", diff)
  }
  */
  if !reflect.DeepEqual(tr, want) {
    t.Errorf("got:\n%s", tr)
    t.Errorf("want:\n%s", want)
  }
}
