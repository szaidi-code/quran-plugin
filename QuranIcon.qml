import QtQuick
import QtQuick.Shapes

Item {
    id: root

    property real iconSize: 16
    property color color: "#ffffff"

    width: iconSize
    height: iconSize
    implicitWidth: iconSize
    implicitHeight: iconSize

    function alpha(c, a) {
        return Qt.rgba(c.r, c.g, c.b, a);
    }

    Shape {
        id: shape
        anchors.fill: parent
        antialiasing: true
        layer.enabled: true
        layer.samples: 4

        readonly property real w: width
        readonly property real h: height

        // 1. Bottom Back Cover Rim
        ShapePath {
            fillColor: root.alpha(root.color, 0.35)
            strokeColor: root.color
            strokeWidth: Math.max(0.8, shape.w * 0.04)
            joinStyle: ShapePath.RoundJoin
            capStyle: ShapePath.RoundCap

            startX: shape.w * 0.15
            startY: shape.h * 0.88
            PathLine { x: shape.w * 0.84; y: shape.h * 0.88 }
        }

        // 2. Page Block at Bottom (book thickness)
        ShapePath {
            fillColor: root.alpha(root.color, 0.22)
            strokeColor: root.color
            strokeWidth: Math.max(0.9, shape.w * 0.045)
            joinStyle: ShapePath.RoundJoin
            capStyle: ShapePath.RoundCap

            startX: shape.w * 0.22
            startY: shape.h * 0.80
            PathLine { x: shape.w * 0.22; y: shape.h * 0.86 }
            PathLine { x: shape.w * 0.82; y: shape.h * 0.86 }
            PathLine { x: shape.w * 0.82; y: shape.h * 0.80 }
        }

        // 2b. Page texture lines (sheets)
        ShapePath {
            fillColor: "transparent"
            strokeColor: root.alpha(root.color, 0.5)
            strokeWidth: Math.max(0.6, shape.w * 0.03)
            capStyle: ShapePath.RoundCap

            startX: shape.w * 0.52
            startY: shape.h * 0.83
            PathLine { x: shape.w * 0.76; y: shape.h * 0.83 }
        }

        // 3. Hanging Bookmark Ribbon (with notched swallowtail)
        ShapePath {
            fillColor: root.color
            strokeColor: root.color
            strokeWidth: Math.max(0.7, shape.w * 0.03)
            joinStyle: ShapePath.MiterJoin

            startX: shape.w * 0.32
            startY: shape.h * 0.79
            PathLine { x: shape.w * 0.32; y: shape.h * 0.98 }
            PathLine { x: shape.w * 0.41; y: shape.h * 0.91 }
            PathLine { x: shape.w * 0.50; y: shape.h * 0.98 }
            PathLine { x: shape.w * 0.50; y: shape.h * 0.79 }
            PathLine { x: shape.w * 0.32; y: shape.h * 0.79 }
        }

        // 4. Front Cover & Spine Outer Body
        ShapePath {
            fillColor: root.alpha(root.color, 0.16)
            strokeColor: root.color
            strokeWidth: Math.max(1.1, shape.w * 0.06)
            joinStyle: ShapePath.RoundJoin
            capStyle: ShapePath.RoundCap

            startX: shape.w * 0.26
            startY: shape.h * 0.07
            PathLine { x: shape.w * 0.84; y: shape.h * 0.07 }
            PathLine { x: shape.w * 0.84; y: shape.h * 0.80 }
            PathLine { x: shape.w * 0.26; y: shape.h * 0.80 }
            PathArc {
                x: shape.w * 0.15
                y: shape.h * 0.72
                radiusX: shape.w * 0.08
                radiusY: shape.h * 0.08
            }
            PathLine { x: shape.w * 0.15; y: shape.h * 0.15 }
            PathArc {
                x: shape.w * 0.26
                y: shape.h * 0.07
                radiusX: shape.w * 0.08
                radiusY: shape.h * 0.08
            }
        }

        // 5. Spine Crease Line
        ShapePath {
            fillColor: "transparent"
            strokeColor: root.color
            strokeWidth: Math.max(0.9, shape.w * 0.045)
            capStyle: ShapePath.RoundCap

            startX: shape.w * 0.27
            startY: shape.h * 0.09
            PathLine { x: shape.w * 0.27; y: shape.h * 0.78 }
        }

        // 6. Cover Inset Frame
        ShapePath {
            fillColor: "transparent"
            strokeColor: root.alpha(root.color, 0.4)
            strokeWidth: Math.max(0.7, shape.w * 0.035)

            startX: shape.w * 0.35
            startY: shape.h * 0.14
            PathLine { x: shape.w * 0.77; y: shape.h * 0.14 }
            PathLine { x: shape.w * 0.77; y: shape.h * 0.73 }
            PathLine { x: shape.w * 0.35; y: shape.h * 0.73 }
            PathLine { x: shape.w * 0.35; y: shape.h * 0.14 }
        }

        // 7. Corner Ornaments (Top-Right & Bottom-Right)
        ShapePath {
            fillColor: root.color
            strokeWidth: 0

            startX: shape.w * 0.67
            startY: shape.h * 0.14
            PathLine { x: shape.w * 0.77; y: shape.h * 0.14 }
            PathLine { x: shape.w * 0.77; y: shape.h * 0.24 }
            PathLine { x: shape.w * 0.67; y: shape.h * 0.14 }
        }
        ShapePath {
            fillColor: root.color
            strokeWidth: 0

            startX: shape.w * 0.67
            startY: shape.h * 0.73
            PathLine { x: shape.w * 0.77; y: shape.h * 0.73 }
            PathLine { x: shape.w * 0.77; y: shape.h * 0.63 }
            PathLine { x: shape.w * 0.67; y: shape.h * 0.73 }
        }

        // 8. Central Islamic Cartouche (Medallion)
        ShapePath {
            fillColor: root.alpha(root.color, 0.28)
            strokeColor: root.color
            strokeWidth: Math.max(0.8, shape.w * 0.04)
            joinStyle: ShapePath.RoundJoin

            startX: shape.w * 0.56
            startY: shape.h * 0.26
            PathQuad {
                x: shape.w * 0.70
                y: shape.h * 0.435
                controlX: shape.w * 0.70
                controlY: shape.h * 0.30
            }
            PathQuad {
                x: shape.w * 0.56
                y: shape.h * 0.61
                controlX: shape.w * 0.70
                controlY: shape.h * 0.57
            }
            PathQuad {
                x: shape.w * 0.42
                y: shape.h * 0.435
                controlX: shape.w * 0.42
                controlY: shape.h * 0.57
            }
            PathQuad {
                x: shape.w * 0.56
                y: shape.h * 0.26
                controlX: shape.w * 0.42
                controlY: shape.h * 0.30
            }
        }

        // 9. Inner Medallion Circle
        ShapePath {
            fillColor: root.color
            strokeColor: root.color
            strokeWidth: Math.max(0.7, shape.w * 0.03)

            startX: shape.w * 0.56
            startY: shape.h * 0.37
            PathArc {
                x: shape.w * 0.56
                y: shape.h * 0.50
                radiusX: shape.w * 0.065
                radiusY: shape.h * 0.065
            }
            PathArc {
                x: shape.w * 0.56
                y: shape.h * 0.37
                radiusX: shape.w * 0.065
                radiusY: shape.h * 0.065
            }
        }

        // 10. Top & Bottom Medallion Accent Diamonds
        ShapePath {
            fillColor: root.color
            strokeWidth: 0

            startX: shape.w * 0.56
            startY: shape.h * 0.29
            PathLine { x: shape.w * 0.58; y: shape.h * 0.315 }
            PathLine { x: shape.w * 0.56; y: shape.h * 0.34 }
            PathLine { x: shape.w * 0.54; y: shape.h * 0.315 }
            PathLine { x: shape.w * 0.56; y: shape.h * 0.29 }
        }
        ShapePath {
            fillColor: root.color
            strokeWidth: 0

            startX: shape.w * 0.56
            startY: shape.h * 0.53
            PathLine { x: shape.w * 0.58; y: shape.h * 0.555 }
            PathLine { x: shape.w * 0.56; y: shape.h * 0.58 }
            PathLine { x: shape.w * 0.54; y: shape.h * 0.555 }
            PathLine { x: shape.w * 0.56; y: shape.h * 0.53 }
        }
    }
}
