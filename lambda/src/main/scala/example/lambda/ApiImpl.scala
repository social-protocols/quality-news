package example.lambda

import example.api.Api
import example.api.Story

import funstack.backend.Fun
import funstack.lambda.apigateway.Handler

import sloth.Client
import cats.effect.IO
import cats.data.Kleisli
import cats.implicits._

import java.nio.ByteBuffer
import boopickle.Default._
import chameleon.ext.boopickle._
import collection.mutable

object Database {
  val stories = mutable.HashMap.empty[Int, Story]
  val upvotes = mutable.HashSet.empty[(String, Int)]
}

object ApiImpl extends Api[Handler.IOKleisli] {
  private val client = Client.contra(Fun.ws.sendTransport[ByteBuffer])

  def getFrontpage = Kleisli { req =>
    // val userId = req.auth.map(_.sub)
    // val userAttrs = userId.traverse(Fun.auth.getUser(_))

    IO(
      Database.stories.values.toList,
    )

  }

  def upvote(storyId: Int) = Kleisli { req =>
    val userIdOpt: Option[String] = req.auth.map(_.sub)
    // val userAttrs = userId.traverse(Fun.auth.getUser(_))

    userIdOpt.fold(IO(println("not logged in"))) { userId =>
      IO {
        println(s"user $userId upvoting $storyId")
        Database.upvotes += (userId -> storyId)
      }
    }

  }

  def submit() = Kleisli { req =>
    val randomStory = Story(
      Math.abs(new scala.util.Random().nextInt()),
      scala.util.Random.alphanumeric.take(10).mkString,
      "http://google.com",
    )

    Database.stories += (randomStory.id -> randomStory)

    IO.unit
  }
}
