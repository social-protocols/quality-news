package example.webapp

import colibri.Subject
import colibri.router._

sealed trait Page {
  final def href = outwatch.dsl.href := s"#${Page.toPath(this).pathString}"
}

object Page {
  case object Home extends Page
  case object Api  extends Page

  object Paths {
    val Home = Root
    val Api  = Root / "api"
  }

  val fromPath: Path => Page = {
    case Paths.Api => Page.Api
    case _         => Page.Home
  }

  val toPath: Page => Path = {
    case Page.Api  => Paths.Api
    case Page.Home => Paths.Home
  }

  val current: Subject[Page] = Router.path
    .imapSubject[Page](Page.toPath)(Page.fromPath)
}
