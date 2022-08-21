package example.webapp

import cats.effect.{IO, IOApp}
import outwatch.Outwatch

import funstack.web.Fun

object Main extends IOApp.Simple {
  LoadCss()

  override def run =
    Fun.ws.start &> Outwatch.renderInto[IO]("#app", App.layout)
}
