package example.webapp

import scala.scalajs.js
import scala.scalajs.js.annotation.JSImport

//TODO: https://github.com/scalacenter/scalajs-bundler/issues/414
// js.import("src/main/css/index.css")
object LoadCss {
  @js.native
  @JSImport("src/main/css/index.css", JSImport.Namespace)
  object Css extends js.Object

  def apply(): Unit = {
    Css
    ()
  }

}
