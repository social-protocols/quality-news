package example.webapp

import example.api.Api
import colibri.Subject
import outwatch.VModifier
import outwatch.dsl._
import funstack.web.tapir
import cats.effect.IO

object Components {

  def websocketRpcApi = {
    val currentRandomNumber = Subject.behavior[Option[Int]](None)

    div(
      button("submit", onClick.doEffect(WsClient.api.submit())),
      WsClient.api.getFrontpage.map(list =>
        list.map(story =>
          div(
            button("▲", onClick.doEffect(WsClient.api.upvote(story.id))),
            a(story.title, href := story.url),
          ),
        ),
      ),
    )
  }

}
